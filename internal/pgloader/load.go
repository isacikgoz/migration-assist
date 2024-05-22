package pgloader

import (
	"embed"
	"fmt"
	"io"
	"os"
	"regexp"
	"text/template"
)

//go:embed templates
var assets embed.FS

type paramters struct {
	MySQLUser     string
	MySQLPassword string
	MySQLAddress  string
	SourceSchema  string

	PGUser       string
	PGPassword   string
	PGAddress    string
	TargetSchema string

	DropFTIndexes bool
}

type PgLoaderConfig struct {
	MySQLDSN    string
	PostgresDSN string

	DropFullTextIndexes bool
}

func GenerateConfigurationFile(output, product string, config PgLoaderConfig) error {
	var f string
	switch product {
	case "boards":
		f = "boards"
	case "playbooks":
		f = "playbooks"
	default:
		f = "config"
	}
	bytes, err := assets.ReadFile(fmt.Sprintf("templates/%s.tmpl", f))
	if err != nil {
		return fmt.Errorf("could not read configuration template: %w", err)
	}

	templ, err := template.New("cfg").Parse(string(bytes))
	if err != nil {
		return fmt.Errorf("could not parse template: %w", err)
	}

	params := paramters{
		DropFTIndexes: config.DropFullTextIndexes,
	}
	err = parseMySQL(&params, config.MySQLDSN)
	if err != nil {
		return fmt.Errorf("could not parse mysql DSN: %w", err)
	}

	err = parsePostgres(&params, config.PostgresDSN)
	if err != nil {
		return fmt.Errorf("could not parse postgres DSN: %w", err)
	}

	var writer io.Writer
	switch output {
	case "":
		writer = os.Stdout
	default:
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer f.Close()
		writer = f
	}

	err = templ.Execute(writer, params)
	if err != nil {
		return fmt.Errorf("error during executing the template: %w", err)
	}

	return nil
}

func parseMySQL(params *paramters, dsn string) error {
	regex := regexp.MustCompile(`^(?P<user>[^:]+):(?P<password>[^@]+)@tcp\((?P<address>[^:]+):(?P<port>\d+)\)\/(?P<database>.+)$`)
	match := regex.FindStringSubmatch(dsn)

	if len(match) > 0 {
		user := match[regex.SubexpIndex("user")]
		password := match[regex.SubexpIndex("password")]
		address := match[regex.SubexpIndex("address")]
		port := match[regex.SubexpIndex("port")]
		database := match[regex.SubexpIndex("database")]

		params.MySQLAddress = fmt.Sprintf("%s:%s", address, port)
		params.MySQLUser = user
		params.MySQLPassword = password
		params.SourceSchema = database
	} else {
		return fmt.Errorf("no match found")
	}
	return nil
}

func parsePostgres(params *paramters, dsn string) error {
	regex := regexp.MustCompile(`^postgres:\/\/(?P<user>[^:]+):(?P<password>[^@]+)@(?P<address>[^:]+):(?P<port>\d+)\/(?P<database>[^\?]+)`)
	match := regex.FindStringSubmatch(dsn)

	if len(match) > 0 {
		user := match[regex.SubexpIndex("user")]
		password := match[regex.SubexpIndex("password")]
		address := match[regex.SubexpIndex("address")]
		port := match[regex.SubexpIndex("port")]
		database := match[regex.SubexpIndex("database")]

		params.PGAddress = fmt.Sprintf("%s:%s", address, port)
		params.PGUser = user
		params.PGPassword = password
		params.TargetSchema = database
	} else {
		return fmt.Errorf("no match found")
	}
	return nil
}
