package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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
		Use:     "mysql",
		Short:   "Checks the MySQL database schema whether it is ready for the migration",
		RunE:    runSourceCheckCmdF,
		Example: "  Rmigration-assist mysql \"root:mostest@tcp(localhost:3306)/mattermost_test\" \\\n--fix-unicode",
		Args:    cobra.MinimumNArgs(1),
	}

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

func runSourceCheckCmdF(cmd *cobra.Command, args []string) error {
	baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})
	var verboseLogger logger.LogInterface

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		verboseLogger = baseLogger
	} else {
		verboseLogger = logger.NewNopLogger()
	}

	mysqlDB, err := store.NewStore("mysql", args[0])
	if err != nil {
		return err
	}
	defer mysqlDB.Close()

	baseLogger.Println("pinging mysql...")
	err = mysqlDB.Ping()
	if err != nil {
		return fmt.Errorf("could not ping mysql: %w", err)
	}
	baseLogger.Println("connected to mysql successfully...")

	fullSchema, _ := cmd.Flags().GetBool("full-schema-check")
	if fullSchema {
		mmVersion, _ := cmd.Flags().GetString("mattermost-version")
		v, err2 := semver.ParseTolerant(mmVersion)
		if err2 != nil {
			return fmt.Errorf("could not parse version: %w", err2)
		}

		tempDir, err3 := os.MkdirTemp("", "mattermost")
		if err3 != nil {
			return fmt.Errorf("could not create temp directory: %w", err3)
		}

		migrationsDir, _ := cmd.Flags().GetString("migrations-dir")
		saveDiff, _ := cmd.Flags().GetBool("save-diff")

		err = runFullSchemaCheck(mysqlDB, migrationsDir, tempDir, v, baseLogger, verboseLogger, saveDiff)
		if err != nil {
			return fmt.Errorf("error during full schema check: %w", err)
		}
	}

	// create procedures
	cleanUpFn, err := createProcedures(mysqlDB, baseLogger)
	if err != nil {
		return fmt.Errorf("error during creating procedures for mysql: %w", err)
	}
	defer cleanUpFn()

	// run MySQL schema checks
	fixArtifacts, _ := cmd.Flags().GetBool("fix-artifacts")

	err = runChecksForMySQL(mysqlDB, "artifacts", fixArtifacts, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running artifact checks for mysql: %w", err)
	}

	fixUnicode, _ := cmd.Flags().GetBool("fix-unicode")

	err = runChecksForMySQL(mysqlDB, "unicode", fixUnicode, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running unicode checks for mysql: %w", err)
	}

	fixVarchar, _ := cmd.Flags().GetBool("fix-varchar")

	err = runChecksForMySQL(mysqlDB, "varchar", fixVarchar, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running varchar checks for mysql: %w", err)
	}

	err = runChecksForMySQL(mysqlDB, "varchar-extended", fixVarchar, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running varchar checks for mysql: %w", err)
	}

	return nil
}

func createProcedures(db *store.DB, baseLogger logger.LogInterface) (func(), error) {
	assets := queries.Assets()

	procedures, err := assets.ReadDir("procedures")
	if err != nil {
		return nil, err
	}

	for _, procedure := range procedures {
		if !strings.HasPrefix(procedure.Name(), "create") {
			continue
		}
		b, err := assets.ReadFile(filepath.Join("procedures", procedure.Name()))
		if err != nil {
			baseLogger.Printf("could not read embedded sql file: %s", err)
		}
		err = db.ExecQuery(context.TODO(), string(b))
		if err != nil {
			baseLogger.Printf("error during creating procedures: %s", err)
		}
	}

	cleanUpFn := func() {
		for _, procedure := range procedures {
			if !strings.HasPrefix(procedure.Name(), "drop") {
				continue
			}
			b, err := assets.ReadFile(filepath.Join("procedures", procedure.Name()))
			if err != nil {
				baseLogger.Printf("could not read embedded sql file: %s", err)
			}
			err = db.ExecQuery(context.TODO(), string(b))
			if err != nil {
				baseLogger.Printf("error during dropping procedures: %s", err)
			}
		}
	}

	return cleanUpFn, nil
}

func runChecksForMySQL(db *store.DB, checkType string, fix bool, baseLogger, verboseLogger logger.LogInterface) error {
	assets := queries.Assets()

	checks, err := assets.ReadDir(filepath.Join("checks", checkType))
	if err != nil {
		return err
	}

	var fixRequired, totalCheck int
	baseLogger.Printf("running checks for %s...\n", checkType)
	for _, artifact := range checks {
		if !strings.HasPrefix(artifact.Name(), "check") {
			continue
		}
		name := stripQueryName(artifact.Name())
		b, err := assets.ReadFile(filepath.Join("checks", checkType, artifact.Name()))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}
		verboseLogger.Printf("checking %s...", name)
		count, err := db.RunSelectCountQuery(context.TODO(), string(b))
		if err != nil {
			return fmt.Errorf("error during running checks: %w", err)
		}
		totalCheck++
		if count == 0 {
			verboseLogger.Printf("%s is okay", name)
			continue
		}
		fixRequired++

		baseLogger.Printf("a fix is required for: %s\n", name)
		if !fix {
			continue
		}

		fixQ, err := assets.ReadFile(filepath.Join("fixes", checkType, "fix_"+strings.TrimPrefix(artifact.Name(), "check_")))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}

		err = db.ExecQuery(context.TODO(), string(fixQ))
		if err != nil {
			return fmt.Errorf("error while trying to fix %s error: %w", name, err)
		}
		baseLogger.Println("the fix query has been executed successfully.")
		fixRequired--
	}

	if fixRequired == 0 {
		baseLogger.Printf("%d checks been made, all good for %s\n", totalCheck, checkType)
	} else {
		baseLogger.Printf("%d checks been made, %d fix(es) is required for %s\n", totalCheck, fixRequired, checkType)
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

func runFullSchemaCheck(db *store.DB, migrationsDir, tempDir string, v semver.Version, baseLogger, verboseLogger logger.LogInterface, saveDiff bool) error {
	ctx := context.Background()

	var mysqlContainer *module.MySQLContainer
	var err error

	baseLogger.Println("setting up a test MySQL instance...")
	mysqlContainer, err = module.RunContainer(ctx,
		// TODO: get version from user database
		testcontainers.WithImage("mysql:8.0.36"),
		testcontainers.WithLogger(verboseLogger),
		module.WithDatabase("foo"),
		module.WithDefaultCredentials(),
	)
	if err != nil {
		log.Fatalf("failed to start container: %s", err)
	}
	defer func() {
		verboseLogger.Println("terminating test container...")

		if err2 := mysqlContainer.Terminate(ctx); err2 != nil {
			log.Fatalf("failed to terminate container: %s", err2)
		}
	}()

	connectionString, err := mysqlContainer.ConnectionString(ctx, "multiStatements=true", "tls=skip-verify")
	if err != nil {
		log.Fatalf("failed to get connection string of container: %s", err)
	}

	dir := "mysql"
	if migrationsDir == "" {
		baseLogger.Printf("cloning %s@%s\n", "repository", v.String())
		err = git.CloneMigrations(git.CloneOptions{
			TempRepoPath: tempDir,
			Output:       dir,
			DriverType:   "mysql",
			Version:      v,
		}, verboseLogger)
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

	baseLogger.Println("running migrations...")
	engine, err := morph.New(context.TODO(), driver, src, morph.WithLogger(logger.NewNopLogger()))
	if err != nil {
		return fmt.Errorf("could not initialize morph: %w", err)
	}

	err = engine.ApplyAll()
	if err != nil {
		return fmt.Errorf("could not apply migrations: %w", err)
	}
	baseLogger.Println("migrations applied.")

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
		if err2 := rows.Scan(&table); err2 != nil {
			return fmt.Errorf("error while scanning tables from test db: %w", err2)
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

	baseLogger.Println("comparing tables...")
	var diffTables int
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
			diffTables++

			if !saveDiff {
				baseLogger.Printf("%s table is not as expected. Diff:\n%s\n", table, diff)
				continue
			}
			verboseLogger.Printf("%s table differs from what is expected.\n", table)

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
	if diffTables == 0 {
		verboseLogger.Printf("MySQL tables are equal from what is expected.\n")
	}

	return nil
}
