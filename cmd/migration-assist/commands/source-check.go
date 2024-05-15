package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/spf13/cobra"
	"github.com/testcontainers/testcontainers-go"

	"github.com/mattermost/morph"
	morph_mysql "github.com/mattermost/morph/drivers/mysql"
	"github.com/mattermost/morph/sources/file"
	module "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/isacikgoz/migration-assist/internal/git"
	"github.com/isacikgoz/migration-assist/internal/logger"
	"github.com/isacikgoz/migration-assist/internal/store"
	"github.com/isacikgoz/migration-assist/queries"
)

func SourceCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "source-check",
		Short:   "Checks the MySQL database schema whether it is ready for the migration",
		RunE:    runSourceCheckCmdF,
		Example: "migration-assist source-check \\\n--mysql=\"root:mostest@tcp(localhost:3306)/mattermost_test\" \\\n--fix-unicode",
	}

	// Required flags
	cmd.Flags().String("mysql", "", "DSN for MySQL")
	_ = cmd.MarkFlagRequired("mysql")

	// Optional flags
	cmd.Flags().Bool("fix-artifacts", false, "Removes the artifacts from older versions of Mattermost")
	cmd.Flags().Bool("fix-varchar", false, "Removes the rows with varchar overflow")
	cmd.Flags().Bool("fix-unicode", false, "Removes the unsupported unicode characters from MySQL tables")
	cmd.Flags().Bool("full-schema-check", false, "Checks the MySQL schema to determine whether it's in desired state")
	cmd.Flags().Bool("save-diff", false, "Writes diffs to files")
	cmd.Flags().String("migrations-dir", "", "Migrations directory (should be used if mattermost-version is not supplied)")
	cmd.Flags().String("mattermost-version", "v9.7", "Mattermost version to be cloned to run migrations")

	return cmd
}

func runSourceCheckCmdF(cmd *cobra.Command, _ []string) error {
	// ping mysql
	mysqlDSN, _ := cmd.Flags().GetString("mysql")
	verbose, _ := cmd.Flags().GetBool("verbose")

	mysqlDB, err := store.NewStore("mysql", mysqlDSN)
	if err != nil {
		return err
	}
	defer mysqlDB.Close()

	fmt.Println("pinging mysql...")
	err = mysqlDB.Ping()
	if err != nil {
		return fmt.Errorf("could not ping mysql: %w", err)
	}
	fmt.Println("connected to mysql successfully...")

	fullSchema, _ := cmd.Flags().GetBool("full-schema-check")
	if fullSchema {
		mmVersion, _ := cmd.Flags().GetString("mattermost-version")
		v, err := semver.ParseTolerant(mmVersion)
		if err != nil {
			return fmt.Errorf("could not parse version: %w", err)
		}

		tempDir, err := os.MkdirTemp("", "mattermost")
		if err != nil {
			return fmt.Errorf("could not create temp directory: %w", err)
		}

		migrationsDir, _ := cmd.Flags().GetString("migrations-dir")
		saveDiff, _ := cmd.Flags().GetBool("save-diff")

		err = runFullSchemaCheck(mysqlDB, migrationsDir, tempDir, "mysql", "mysql", v, verbose, saveDiff)
		if err != nil {
			return fmt.Errorf("error during full schema check: %w", err)
		}

	}

	// run MySQL schema checks
	fixArtifacts, _ := cmd.Flags().GetBool("fix-artifacts")

	err = runChecksForMySQL(mysqlDB, "artifacts", fixArtifacts)
	if err != nil {
		return fmt.Errorf("error during running artifact checks for mysql: %w", err)
	}

	fixUnicode, _ := cmd.Flags().GetBool("fix-unicode")

	err = runChecksForMySQL(mysqlDB, "unicode", fixUnicode)
	if err != nil {
		return fmt.Errorf("error during running unicode checks for mysql: %w", err)
	}

	fixVarchar, _ := cmd.Flags().GetBool("fix-varchar")

	err = runChecksForMySQL(mysqlDB, "varchar", fixVarchar)
	if err != nil {
		return fmt.Errorf("error during running varchar checks for mysql: %w", err)
	}

	return nil
}

func runChecksForMySQL(db *store.DB, checkType string, fix bool) error {
	assets := queries.Assets()

	artifacts, err := assets.ReadDir(checkType)
	if err != nil {
		return err
	}

	var fixRequired int
	fmt.Printf("running checks for %s...\n", checkType)
	for _, artifact := range artifacts {
		if !strings.HasPrefix(artifact.Name(), "check") {
			continue
		}
		name := stripQueryName(artifact.Name())
		b, err := assets.ReadFile(filepath.Join(checkType, artifact.Name()))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}
		count, err := db.RunSelectCountQuery(context.TODO(), string(b))
		if err != nil {
			return fmt.Errorf("error during running checks: %w", err)
		}

		if count == 0 {
			continue
		}
		fixRequired++

		fmt.Printf("a fix is required for: %s\n", name)
		if !fix {
			continue
		}

		fixQ, err := assets.ReadFile(filepath.Join(checkType, "fix_"+strings.TrimPrefix(artifact.Name(), "check_")))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}

		err = db.ExecQuery(context.TODO(), string(fixQ))
		if err != nil {
			return fmt.Errorf("error while trying to fix %s error: %w", name, err)
		}
		fmt.Println("the fix query has been executed successfully.")
		fixRequired--
	}

	if fixRequired == 0 {
		fmt.Printf("all good for %s\n", checkType)
	}

	return nil
}

func stripQueryName(fileName string) string {
	fileName = strings.TrimPrefix(fileName, "check_")
	fileName = strings.TrimPrefix(fileName, "fix_")
	return strings.TrimSuffix(fileName, ".sql")
}

type CreateTable struct {
	Table       string
	CreateTable string
}

func runFullSchemaCheck(db *store.DB, migrationsDir, tempDir, dbType, dir string, v semver.Version, verbose, saveDiff bool) error {
	ctx := context.Background()

	var mysqlContainer *module.MySQLContainer
	var err error
	fmt.Println("setting up a test MySQL instance...")
	mysqlContainer, err = module.RunContainer(ctx,
		testcontainers.WithImage("mysql:8.0.36"),
		testcontainers.WithLogger(logger.NewNopLogger()),
		module.WithDatabase("foo"),
		module.WithDefaultCredentials(),
	)
	if err != nil {
		log.Fatalf("failed to start container: %s", err)
	}
	defer func() {
		fmt.Println("terminating test container...")

		if err := mysqlContainer.Terminate(ctx); err != nil {
			log.Fatalf("failed to terminate container: %s", err)
		}
	}()

	connectionString, err := mysqlContainer.ConnectionString(ctx, "multiStatements=true", "tls=skip-verify")
	if err != nil {
		log.Fatalf("failed to get connection string of container: %s", err)
	}

	if migrationsDir == "" {
		err = git.CloneMigrations(git.CloneOptions{
			TempRepoPath: tempDir,
			Output:       dir,
			DriverType:   dbType,
			Version:      v,
		}, verbose)
		if err != nil {
			return fmt.Errorf("error during cloning migrations: %w", err)
		}
	} else {
		dir = migrationsDir
	}

	// create mysql connection
	testDB, err := store.NewStore("mysql", connectionString)
	if err != nil {
		return err
	}
	defer testDB.Close()

	// run the migrations
	driver, err := morph_mysql.WithInstance(testDB.GetDB())
	if err != nil {
		return fmt.Errorf("could not initialize driver: %w", err)
	}

	src, err := file.Open(dir)
	if err != nil {
		return fmt.Errorf("could not read migrations: %w", err)
	}

	fmt.Println("running migrations...")
	engine, err := morph.New(context.TODO(), driver, src, morph.WithLogger(logger.NewNopLogger()))
	if err != nil {
		return fmt.Errorf("could not initialize morph: %w", err)
	}

	err = engine.ApplyAll()
	if err != nil {
		return fmt.Errorf("could not apply migrations: %w", err)
	}
	fmt.Println("migrations applied.")

	testConn, err := testDB.GetDB().Conn(context.TODO())
	if err != nil {
		return fmt.Errorf("could not grab connection from test db: %w", err)
	}

	tables := make([]string, 0)
	rows, err := testConn.QueryContext(context.TODO(), "SHOW TABLES")
	if err != nil {
		return fmt.Errorf("could notget tables test db: %w", err)
	}

	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("error while scanning tables from test db: %w", err)
		}
		tables = append(tables, table)
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("error during query: %w", err)
	}

	actualConn, err := db.GetDB().Conn(context.TODO())
	if err != nil {
		return fmt.Errorf("could not grab connection from actual db: %w", err)
	}

	fmt.Println("comparing tables...")
	for _, table := range tables {
		row := testConn.QueryRowContext(context.TODO(), fmt.Sprintf("SHOW CREATE TABLE %s", table))
		var expected CreateTable
		err = row.Scan(&expected.Table, &expected.CreateTable)
		if err != nil {
			return fmt.Errorf("could not get table definition from test db: %w", err)
		}
		var actual CreateTable
		row = actualConn.QueryRowContext(context.TODO(), fmt.Sprintf("SHOW CREATE TABLE %s", table))
		err = row.Scan(&actual.Table, &actual.CreateTable)
		if err != nil {
			return fmt.Errorf("could not get table definition from actual db: %w", err)
		}

		diff := git.Diff(actual.CreateTable, expected.CreateTable)
		if diff != "" {
			time.Sleep(500 * time.Millisecond)

			if !saveDiff {
				fmt.Printf("%s table is not as expected. Diff:\n%s\n", table, diff)
				continue
			}
			fmt.Printf("%s table differs from what is expected.\n", table)

			_ = os.RemoveAll("diffs")
			err = os.MkdirAll("diffs", 0750)
			if err != nil {
				return fmt.Errorf("could not create diff directory: %w", err)
			}

			err := os.WriteFile(filepath.Join("diffs", table+".diff"), []byte(diff), 0644)
			if err != nil && !os.IsExist(err) {
				return fmt.Errorf("could not create diff directory: %w", err)
			}
		}
	}

	return nil
}
