package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/isacikgoz/migration-assist/internal/store"
	"github.com/mattermost/morph"
	"github.com/mattermost/morph/drivers/postgres"
	"github.com/mattermost/morph/sources/file"
	"github.com/spf13/cobra"
)

const (
	mattermostRepositoryURL    = "https://github.com/mattermost/mattermost.git"
	defaultMigrationsDirectory = "migrations"
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

		if _, err := os.Stat(defaultMigrationsDirectory); err == nil || os.IsExist(err) {
			return fmt.Errorf("migrations directory already exists")
		}

		tempDir, err := os.MkdirTemp("", "mattermost")
		if err != nil {
			return fmt.Errorf("could not create temp directory: %w", err)
		}

		gitBinary, _ := cmd.Flags().GetString("git")

		err = cloneMigrations(gitBinary, tempDir, v)
		if err != nil {
			return fmt.Errorf("error during cloning migrations: %w", err)
		}

		migrationDir = defaultMigrationsDirectory
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
	engine, err := morph.New(context.TODO(), driver, src)
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

func cloneMigrations(gitBinary, repositoryPath string, version semver.Version) error {
	// 1. first check if the git installedd
	cmd := exec.Command(gitBinary, "--version")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("git binary is not installed :%w", err)
	}

	// 2. clone the repository
	gitArgs := []string{"clone", "--no-checkout", "--depth=1", "--filter=tree:0", fmt.Sprintf("--branch=%s", fmt.Sprintf("v%s", version.String())), mattermostRepositoryURL, repositoryPath}

	cmd = exec.Command("git", gitArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during clone: %w", err)
	}

	// 3. download only migration files
	v8 := semver.MustParse("8.0.0")
	migrationsDir := filepath.Join("server", "channels", "db", "migrations", "postgres")
	if version.LT(v8) {
		migrationsDir = strings.TrimPrefix(migrationsDir, filepath.Join("server", "channels"))
	}

	cmd = exec.Command(gitBinary, "sparse-checkout", "set", "--no-cone", migrationsDir)
	cmd.Dir = repositoryPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during sparse checkout: %w", err)
	}

	cmd = exec.Command(gitBinary, "checkout")
	cmd.Dir = repositoryPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during checkout: %w", err)
	}

	// 4. move files to migrations directory and remove temp dir
	err = os.Rename(filepath.Join(repositoryPath, migrationsDir), defaultMigrationsDirectory)
	if err != nil {
		return fmt.Errorf("error while renaming migrations directory: %w", err)
	}

	err = os.RemoveAll(repositoryPath)
	if err != nil {
		return fmt.Errorf("error while removing temporary directory: %w", err)
	}

	return nil
}
