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
		newPermSetCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func addPathFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("path", "p", "", "Path to the Salesforce metadata directory, e.g. force-app/main/default (required)")
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
	repo := false
	export := ""
	exportOut := ""
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare profiles and report missing permissions",
		Long: `Compares profiles and reports tags present in some profiles but missing from others.
Tags present in every profile are suppressed (shared). Use --details to also show
permission value differences for shared tags. Use --repo to cross-reference profile
permissions against the filesystem. Use --export or --export-out to write the report
to a file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("path")

			if projectPath == "" {
				return fmt.Errorf("required flag --path / -p is missing")
			}

			// Everything below touches the filesystem. A failure there is
			// not a usage error, so stop repeating the flag list.
			cmd.SilenceUsage = true

			g, err := schema.LoadProfiles(projectPath)
			if err != nil {
				return err
			}

			for _, decodeErr := range g.DecodeErrors() {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", decodeErr)
			}

			if len(g.ProfileNodes) < 2 {
				return fmt.Errorf("diff requires at least 2 profiles, found %d", len(g.ProfileNodes))
			}

			repoPath := ""
			if repo {
				repoPath = projectPath
			}

			report := reportpkg.ComputeDiff(g, details, repoPath)

			if export != "" || exportOut != "" {
				exportFmt := export
				if exportFmt == "" {
					exportFmt = "json" // file default
				}

				outPath := exportOut

				if outPath == "" {
					ext := "json"
					if exportFmt == "text" {
						ext = "txt"
					}

					outPath = deriveExportBase(projectPath) + ".diff." + ext
				}

				var content string
				switch exportFmt {
				case "json":
					out, err := json.MarshalIndent(report, "", "  ")
					if err != nil {
						return fmt.Errorf("marshal diff report: %w", err)
					}
					content = string(out)
				case "text":
					content = reportpkg.FormatDiffText(report)
				default:
					return fmt.Errorf("unsupported export format: %s (supported: json, text)", exportFmt)
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
	cmd.Flags().BoolVar(&repo, "repo", false, "Cross-reference profile permissions against the filesystem")
	cmd.Flags().StringVarP(&export, "export", "e", "", "Export the report to a file; format is json or text (default: json)")
	cmd.Flags().StringVar(&exportOut, "export-out", "", "Output path for the export file (auto-derived when empty; implies --export)")
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

			// Everything below touches the filesystem. A failure there is
			// not a usage error, so stop repeating the flag list.
			cmd.SilenceUsage = true

			if err := normalizer.WriteProfiles(g); err != nil {
				return err
			}

			// A profile that failed to decode was skipped by the loader and
			// left untouched. Saving the rest is still useful, but the run must
			// not report success.
			if decodeErrs := g.DecodeErrors(); len(decodeErrs) > 0 {
				return fmt.Errorf("%d profile(s) could not be decoded and were left untouched: %w", len(decodeErrs), decodeErrs[0])
			}

			fmt.Fprintf(os.Stderr, "Done. Profiles saved.\n")
			return nil
		},
	}
	addPathFlag(cmd)
	cmd.Flags().BoolVarP(&useOrgSchema, "use-org-schema", "s", false, "Cross-check metadata against Salesforce org schema before saving")
	cmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Salesforce org alias (default: SF CLI default org)")
	cmd.Flags().StringVarP(&excludeFlag, "exclude", "e", "", "Comma-separated case-insensitive prefixes; matching entries skip normalization and preserve original XML (requires --use-org-schema)")
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

			// Everything below touches the filesystem. A failure there is
			// not a usage error, so stop repeating the flag list.
			cmd.SilenceUsage = true

			return converter.ConvertProfiles(projectPath, profileFlag, profilesFlag)
		},
	}

	addPathFlag(cmd)
	cmd.Flags().StringVarP(&profileFlag, "profile", "r", "", "Convert a single profile by name (mutually exclusive with --profiles)")
	cmd.Flags().StringVarP(&profilesFlag, "profiles", "l", "", "Comma-separated list of profile names to convert (mutually exclusive with --profile)")
	cmd.MarkFlagsMutuallyExclusive("profile", "profiles")
	return cmd
}

func newPermSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permset",
		Short: "Manage Salesforce permission set metadata",
	}
	cmd.AddCommand(
		newPermSetConvertCmd(),
	)
	return cmd
}

func newPermSetConvertCmd() *cobra.Command {
	permsetFlag := ""
	permsetsFlag := ""

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert permission set XML to YAML (1:1, no normalization)",
		Long: `Converts .permissionset-meta.xml files to YAML preserving the original
XML structure exactly — no normalization, no schema cross-check, no sorting.

Output files are written as siblings to the source with a .permissionset-meta.yaml
extension. The source XML files are not modified.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("path")

			if projectPath == "" {
				return fmt.Errorf("required flag --path / -p is missing")
			}

			// Everything below touches the filesystem. A failure there is
			// not a usage error, so stop repeating the flag list.
			cmd.SilenceUsage = true

			return converter.ConvertPermSets(projectPath, permsetFlag, permsetsFlag)
		},
	}

	addPathFlag(cmd)
	cmd.Flags().StringVarP(&permsetFlag, "permset", "r", "", "Convert a single permission set by name (mutually exclusive with --permsets)")
	cmd.Flags().StringVarP(&permsetsFlag, "permsets", "l", "", "Comma-separated list of permission set names to convert (mutually exclusive with --permset)")
	cmd.MarkFlagsMutuallyExclusive("permset", "permsets")
	return cmd
}
