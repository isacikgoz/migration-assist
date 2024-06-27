package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

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

	amCmd := RunAfterMigration()
	amCmd.Flags().String("schema", "public", "the default schema to be used for the session")
	cmd.AddCommand(amCmd)

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

func runPostMigrateCmdF(c *cobra.Command, args []string) error {
	baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})
	schema, _ := c.Flags().GetString("schema")

	postgresDB, err := store.NewStore("postgres", args[0])
	if err != nil {
		return err
	}
	defer postgresDB.Close()

	rows, err := postgresDB.GetDB().QueryContext(c.Context(), "SHOW search_path")
	if err != nil {
		return fmt.Errorf("could not determine the search_path: %w", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var s string
		err = rows.Scan(&s)
		if err != nil {
			return fmt.Errorf("could not scan the schema for search_path: %w", err)
		}
		schemas = append(schemas, s)
	}
	if len(schemas) == 0 {
		return fmt.Errorf("no value available for search_path")
	} else if _, ok := slices.BinarySearch(schemas, schema); !ok {
		baseLogger.Printf("could not find the default schema %q in search_path, consider setting it from the postgresql console\n", schema)
		err = postgresDB.ExecQuery(c.Context(), fmt.Sprintf("SELECT pg_catalog.set_config('search_path', '\"$user\", %s', false)", schema))
		if err != nil {
			return fmt.Errorf("could not set search_path for the session: %w", err)
		}
		baseLogger.Printf("search_path is set to %q for the currrent session\n", schema)
	}

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
