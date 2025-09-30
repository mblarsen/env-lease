package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// convertCmd represents the convert command
var convertCmd = &cobra.Command{
	Use:   "convert [file]",
	Short: "Convert a .env or .envrc file to env-lease.toml format",
	Long: `Parses a .env or .envrc file to scaffold a corresponding env-lease.toml configuration.

The command reads an environment file (defaulting to .envrc or .env if no file is provided)
and generates a valid env-lease.toml file to standard output.
It looks for variable assignments with 1Password secret URIs (e.g., export API_KEY="op://...").
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var input []byte
		var err error
		var filename string

		if len(args) > 0 {
			filename = args[0]
			input, err = ioutil.ReadFile(filename)
			if err != nil {
				return fmt.Errorf("could not read file %s: %w", filename, err)
			}
		} else {
			if _, err := os.Stat(".envrc"); err == nil {
				filename = ".envrc"
				input, err = ioutil.ReadFile(filename)
				if err != nil {
					return fmt.Errorf("could not read .envrc: %w", err)
				}
			} else if _, err := os.Stat(".env"); err == nil {
				filename = ".env"
				input, err = ioutil.ReadFile(filename)
				if err != nil {
					return fmt.Errorf("could not read .env: %w", err)
				}
			} else {
				return fmt.Errorf("no input file specified and no .envrc or .env file found")
			}
		}

		output := convertToToml(string(input), filename)
		cmd.Print(output)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)
}

func convertToToml(input, destination string) string {
	var builder strings.Builder

	re := regexp.MustCompile(`^\s*(?:export\s+)?([\w]+)=["']?(op://[^"']+)["']?`)
	lines := strings.Split(input, "\n")

	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			builder.WriteString("[[lease]]\n")
			builder.WriteString(fmt.Sprintf("variable = \"%s\"\n", matches[1]))
			builder.WriteString(fmt.Sprintf("source = \"%s\"\n", matches[2]))
			builder.WriteString(fmt.Sprintf("destination = \"%s\"\n", destination))
			builder.WriteString("duration = \"1h\"\n")
			builder.WriteString("\n")
		}
	}

	return builder.String()
}
