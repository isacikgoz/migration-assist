package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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

	return cmd
}

func runSourceCheckCmdF(cmd *cobra.Command, _ []string) error {
	// ping mysql
	mysqlDSN, _ := cmd.Flags().GetString("mysql")

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
	fmt.Println("connected to mysql successfully.")

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
			fmt.Printf("all good for: %s\n", name)
			continue
		}

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
	}

	return nil
}

func stripQueryName(fileName string) string {
	fileName = strings.TrimPrefix(fileName, "check_")
	fileName = strings.TrimPrefix(fileName, "fix_")
	return strings.TrimSuffix(fileName, ".sql")
}
