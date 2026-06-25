package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"xml2l/internal/converter"
	"xml2l/internal/normalizer"
	"xml2l/internal/orgschema"
	reportpkg "xml2l/internal/report"
	"xml2l/internal/schema"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "xml2l",
		Short: "Salesforce profile XML manipulation and normalization engine",
	}

	rootCmd.AddCommand(
		newProfileCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func addPathFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("path", "p", "", "Path to the root SFDX project directory (required)")
}

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage Salesforce profile metadata",
	}
	cmd.AddCommand(
		newDiffCmd(),
		newSaveCmd(),
		newConvertCmd(),
	)
	return cmd
}

// deriveExportBase extracts a filesystem-safe base name from the profile path
// for use in auto-generated export filenames.
func deriveExportBase(projectPath string) string {
	clean := strings.TrimRight(projectPath, "/\\")
	base := filepath.Base(clean)
	if base == "" || base == "." {
		base = "profiles"
	}
	return base
}

func newDiffCmd() *cobra.Command {
	details := false
	export := ""
	exportFmt := "text"
	exportOut := ""
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare profiles and report missing permissions",
		Long: `Compares profiles and reports tags present in some profiles but missing from others.
Tags present in every profile are suppressed (shared). Use --details to also show
permission value differences for shared tags. Use --export to write the report to a file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("path")

			if projectPath == "" {
				return fmt.Errorf("required flag --path / -p is missing")
			}

			g, err := schema.LoadProfiles(projectPath)
			if err != nil {
				return err
			}

			if len(g.ProfileNodes) < 2 {
				return fmt.Errorf("diff requires at least 2 profiles, found %d", len(g.ProfileNodes))
			}

			if details {
				hasAdmin := false
				for _, pn := range g.ProfileNodes {
					if pn.Name == "Admin" {
						hasAdmin = true
						break
					}
				}

				if !hasAdmin {
					return fmt.Errorf("--details requires an Admin profile; none found in %s", projectPath)
				}
			}

			report := reportpkg.ComputeDiff(g, details, projectPath)

			exportFmtVal := export
			if export == "" {
				exportFmtVal = exportFmt
			}

			if export != "" || exportFmt != "text" {
				outPath := exportOut

				if outPath == "" {
					base := deriveExportBase(projectPath)
					ext := "json"
					if exportFmtVal == "text" {
						ext = "txt"
					}
					outPath = base + ".diff." + ext
				}

				var content string
				switch exportFmtVal {
				case "json":
					out, err := json.MarshalIndent(report, "", "  ")
					if err != nil {
						return fmt.Errorf("marshal diff report: %w", err)
					}
					content = string(out)
				case "text":
					content = reportpkg.FormatDiffText(report)
				default:
					return fmt.Errorf("unsupported export format: %s (supported: json, text)", exportFmtVal)
				}

				if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
					return fmt.Errorf("write %s: %w", outPath, err)
				}

				fmt.Println(outPath)
				return nil
			}

			fmt.Print(reportpkg.FormatDiffText(report))
			return nil
		},
	}
	addPathFlag(cmd)
	cmd.Flags().BoolVar(&details, "details", false, "Include permission value differences for shared tags")
	cmd.Flags().StringVarP(&export, "export", "e", "", "Export format: json or text (default: text for stdout, json for file)")
	cmd.Flags().StringVar(&exportFmt, "export-format", "text", "Export format when --export is used")
	cmd.Flags().StringVar(&exportOut, "export-out", "", "Output path for the export file (auto-derived when empty)")
	return cmd
}

func newSaveCmd() *cobra.Command {
	useOrgSchema := false
	orgFlag := ""
	excludeFlag := ""
	cmd := &cobra.Command{
		Use:   "save",
		Short: "Normalize and save profile changes to disk",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("path")
			if projectPath == "" {
				return fmt.Errorf("required flag --path / -p is missing")
			}

			g, err := schema.LoadProfiles(projectPath)
			if err != nil {
				return err
			}

			if useOrgSchema {
				os, err := orgschema.Fetch(orgFlag)
				if err != nil {
					return err
				}

				g.SetOrgSchema(os)
			}

			// Exclude flag: skip normalization for entries matching patterns.
			var excludePatterns []string

			if excludeFlag != "" {
				if !useOrgSchema {
					return fmt.Errorf("--exclude / -e requires --use-org-schema / -s")
				}

				for _, term := range strings.Split(excludeFlag, ",") {
					trimmed := strings.TrimSpace(strings.ToLower(term))
					if trimmed != "" {
						excludePatterns = append(excludePatterns, trimmed)
					}
				}

				g.SetExcludePatterns(excludePatterns)
			}

			if err := normalizer.WriteProfiles(g); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Done. Profiles saved.\n")
			return nil
		},
	}
	addPathFlag(cmd)
	cmd.Flags().BoolVarP(&useOrgSchema, "use-org-schema", "s", false, "Cross-check metadata against Salesforce org schema before saving")
	cmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Salesforce org alias (default: SF CLI default org)")
	cmd.Flags().StringVarP(&excludeFlag, "exclude", "e", "", "Comma-separated case-insensitive substrings; matching entries skip normalization and preserve original XML (requires --use-org-schema)")
	return cmd
}

func newConvertCmd() *cobra.Command {
	profileFlag := ""
	profilesFlag := ""

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert profile XML to YAML (1:1, no normalization)",
		Long: `Converts .profile-meta.xml files to YAML preserving the original
XML structure exactly — no normalization, no schema cross-check, no sorting.

Output files are written as siblings to the source with a .profile-meta.yaml
extension. The source XML files are not modified.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("path")

			if projectPath == "" {
				return fmt.Errorf("required flag --path / -p is missing")
			}

			return converter.ConvertProfiles(projectPath, profileFlag, profilesFlag)
		},
	}

	addPathFlag(cmd)
	cmd.Flags().StringVarP(&profileFlag, "profile", "r", "", "Convert a single profile by name (mutually exclusive with --profiles)")
	cmd.Flags().StringVarP(&profilesFlag, "profiles", "l", "", "Comma-separated list of profile names to convert (mutually exclusive with --profile)")
	cmd.MarkFlagsMutuallyExclusive("profile", "profiles")
	return cmd
}
