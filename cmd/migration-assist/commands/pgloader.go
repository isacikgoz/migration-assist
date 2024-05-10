package commands

import (
	"fmt"

	"github.com/isacikgoz/migration-assist/internal/pgloader"
	"github.com/spf13/cobra"
)

func GeneratePgloaderConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gen-pgloader-config",
		Short:   "Generates a pgLoader configuration from DSN values",
		RunE:    runGeneratePgloaderConfigCmdF,
		Example: "migration-assist gen-pgloader-config \\\n--postgres=\"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--mysql=\"root:mostest@tcp(localhost:3306)/mattermost_test\" \\\n--drop-indexes=false",
	}

	// Required flags
	cmd.Flags().String("mysql", "", "DSN for MySQL")
	_ = cmd.MarkFlagRequired("mysql")
	cmd.Flags().String("postgres", "", "DSN for Postgres")
	_ = cmd.MarkFlagRequired("postgres")

	// Optional flags
	cmd.Flags().String("output", "", "The filename of the generated configuration")
	cmd.Flags().Bool("drop-indexes", true, "Adds clauses to drop full-text indexes before the migration")
	return cmd
}

func runGeneratePgloaderConfigCmdF(cmd *cobra.Command, _ []string) error {
	mysqlDSN, _ := cmd.Flags().GetString("mysql")
	postgresDSN, _ := cmd.Flags().GetString("postgres")

	output, _ := cmd.Flags().GetString("output")
	dropIndex, _ := cmd.Flags().GetBool("drop-indexes")

	err := pgloader.GenerateConfigurationFile(output, pgloader.PgLoaderConfig{
		MySQLDSN:            mysqlDSN,
		PostgresDSN:         postgresDSN,
		DropFullTextIndexes: dropIndex,
	})
	if err != nil {
		return fmt.Errorf("could not generate config: %w", err)
	}

	return nil
}
