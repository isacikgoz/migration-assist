package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/blang/semver/v4"
	"github.com/isacikgoz/migration-assist/internal/git"
	"github.com/isacikgoz/migration-assist/internal/logger"
	"github.com/isacikgoz/migration-assist/internal/store"
	"github.com/mattermost/morph"
	"github.com/mattermost/morph/drivers/postgres"
	"github.com/mattermost/morph/sources/file"
	"github.com/spf13/cobra"
)

func TargetCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "target-check",
		Short:   "Checks the Postgres database schema whether it is ready for the migration",
		RunE:    runTargetCheckCmdF,
		Example: "migration-assist target-check \\\n--postgres=\"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--run-migrations",
	}

	// Required flags
	cmd.Flags().String("postgres", "", "DSN for Postgres")
	_ = cmd.MarkFlagRequired("postgres")

	// Optional flags
	cmd.Flags().Bool("run-migrations", false, "Runs migrations for Postgres schema")
	cmd.Flags().String("mattermost-version", "v8.1", "Mattermost version to be cloned to run migrations")
	cmd.Flags().String("migrations-dir", "", "Migrations directory (should be used if mattermost-version is not supplied)")
	cmd.Flags().String("git", "git", "git binary to be executed if the repository will be cloned")

	return cmd
}

func runTargetCheckCmdF(cmd *cobra.Command, _ []string) error {
	// ping postgres
	postgresDSN, _ := cmd.Flags().GetString("postgres")

	verbose, _ := cmd.Flags().GetBool("verbose")

	postgresDB, err := store.NewStore("postgres", postgresDSN)
	if err != nil {
		return err
	}
	defer postgresDB.Close()

	fmt.Println("pinging postgres...")
	err = postgresDB.Ping()
	if err != nil {
		return fmt.Errorf("could not ping postgres: %w", err)
	}
	fmt.Println("connected to postgres successfully.")

	runMigrations, _ := cmd.Flags().GetBool("run-migrations")
	if !runMigrations {
		return nil
	}

	// download required migrations if necessary
	migrationDir, _ := cmd.Flags().GetString("migration-dir")
	if migrationDir == "" {
		mmVersion, _ := cmd.Flags().GetString("mattermost-version")
		v, err := semver.ParseTolerant(mmVersion)
		if err != nil {
			return fmt.Errorf("could not parse version: %w", err)
		}

		tempDir, err := os.MkdirTemp("", "mattermost")
		if err != nil {
			return fmt.Errorf("could not create temp directory: %w", err)
		}

		err = git.CloneMigrations(git.CloneOptions{
			TempRepoPath: tempDir,
			Output:       "postgres",
			DriverType:   "postgres",
			Version:      v,
		}, verbose)
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

	fmt.Println("running migrations..")
	engine, err := morph.New(context.TODO(), driver, src, morph.WithLogger(logger.NewNopLogger()))
	if err != nil {
		return fmt.Errorf("could not initialize morph: %w", err)
	}

	err = engine.ApplyAll()
	if err != nil {
		return fmt.Errorf("could not apply migrations: %w", err)
	}
	fmt.Println("migrations applied.")

	return nil
}
