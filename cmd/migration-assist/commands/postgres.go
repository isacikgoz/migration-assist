package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blang/semver/v4"
	"github.com/isacikgoz/migration-assist/internal/git"
	"github.com/isacikgoz/migration-assist/internal/logger"
	"github.com/isacikgoz/migration-assist/internal/store"
	"github.com/isacikgoz/migration-assist/queries"
	"github.com/mattermost/morph"
	"github.com/mattermost/morph/drivers/postgres"
	"github.com/mattermost/morph/sources/file"
	"github.com/spf13/cobra"
)

func TargetCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "postgres",
		Short:   "Checks the Postgres database schema whether it is ready for the migration",
		RunE:    runTargetCheckCmdF,
		Example: "  migration-assist postgres \"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--run-migrations",
		Args:    cobra.MinimumNArgs(1),
	}

	cmd.AddCommand(RunAfterMigration())

	// Optional flags
	cmd.Flags().Bool("run-migrations", false, "Runs migrations for Postgres schema")
	cmd.Flags().String("mattermost-version", "v8.1", "Mattermost version to be cloned to run migrations")
	cmd.Flags().String("migrations-dir", "", "Migrations directory (should be used if mattermost-version is not supplied)")
	cmd.Flags().String("git", "git", "git binary to be executed if the repository will be cloned")

	return cmd
}

func RunAfterMigration() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "post-migrate",
		Short:   "Creates indexes after the migration is completed",
		RunE:    runPostMigrateCmdF,
		Example: "  migration-assist postgres post-migrate \"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\"",
		Args:    cobra.MinimumNArgs(1),
	}

	return cmd
}

func runTargetCheckCmdF(cmd *cobra.Command, args []string) error {
	baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})
	var verboseLogger logger.LogInterface

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		verboseLogger = baseLogger
	} else {
		verboseLogger = logger.NewNopLogger()
	}

	postgresDB, err := store.NewStore("postgres", args[0])
	if err != nil {
		return err
	}
	defer postgresDB.Close()

	baseLogger.Println("pinging postgres...")
	err = postgresDB.Ping()
	if err != nil {
		return fmt.Errorf("could not ping postgres: %w", err)
	}
	baseLogger.Println("connected to postgres successfully.")

	runMigrations, _ := cmd.Flags().GetBool("run-migrations")
	if !runMigrations {
		return nil
	}

	// download required migrations if necessary
	migrationDir, _ := cmd.Flags().GetString("migration-dir")
	if migrationDir == "" {
		mmVersion, _ := cmd.Flags().GetString("mattermost-version")
		v, err2 := semver.ParseTolerant(mmVersion)
		if err2 != nil {
			return fmt.Errorf("could not parse version: %w", err2)
		}

		tempDir, err3 := os.MkdirTemp("", "mattermost")
		if err3 != nil {
			return fmt.Errorf("could not create temp directory: %w", err3)
		}

		baseLogger.Printf("cloning %s@%s\n", "repository", v.String())
		err = git.CloneMigrations(git.CloneOptions{
			TempRepoPath: tempDir,
			Output:       "postgres",
			DriverType:   "postgres",
			Version:      v,
		}, verboseLogger)
		if err != nil {
			return fmt.Errorf("error during cloning migrations: %w", err)
		}

		migrationDir = "postgres"
	}

	// run the migrations
	driver, err := postgres.WithInstance(postgresDB.GetDB())
	if err != nil {
		return fmt.Errorf("could not initialize driver: %w", err)
	}

	src, err := file.Open(migrationDir)
	if err != nil {
		return fmt.Errorf("could not read migrations: %w", err)
	}

	baseLogger.Println("running migrations..")
	engine, err := morph.New(context.TODO(), driver, src, morph.WithLogger(logger.NewNopLogger()))
	if err != nil {
		return fmt.Errorf("could not initialize morph: %w", err)
	}

	err = engine.ApplyAll()
	if err != nil {
		return fmt.Errorf("could not apply migrations: %w", err)
	}
	baseLogger.Println("migrations applied.")

	return nil
}

func runPostMigrateCmdF(_ *cobra.Command, args []string) error {
	baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})

	postgresDB, err := store.NewStore("postgres", args[0])
	if err != nil {
		return err
	}
	defer postgresDB.Close()

	assets := queries.Assets()
	queries, err := assets.ReadDir("post-migrate")
	if err != nil {
		return err
	}

	baseLogger.Println("running migrations..")

	for _, query := range queries {
		b, err := assets.ReadFile(filepath.Join("post-migrate", query.Name()))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}

		baseLogger.Printf("applying %s\n", query.Name())
		err = postgresDB.ExecQuery(context.TODO(), string(b))
		if err != nil {
			return fmt.Errorf("error during running post-migrate queries: %w", err)
		}
	}

	baseLogger.Println("indexes created.")
	return nil
}
