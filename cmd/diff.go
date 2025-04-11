package cmd

//
//import (
//	"os"
//
//	"github.com/gonvenience/ytbx"
//	"github.com/homeport/dyff/pkg/dyff"
//	"github.com/spf13/cobra"
//	"sigs.k8s.io/kustomize/api/resource"
//)
//
//type DiffFlags struct {
//	short bool
//}
//
//var diffArgs DiffFlags
//
//// getCmd represents the get command
//var diffCmd = &cobra.Command{
//	Use:   "diff",
//	Short: "Diff two flux clusters",
//}
//
//func init() {
//	diffCmd.PersistentFlags().BoolVarP(
//		&getArgs.allNamespaces,
//		"all-namespaces",
//		"A",
//		false,
//		"diff the requested object(s) across all namespaces",
//	)
//	diffCmd.PersistentFlags().StringVarP(
//		&getArgs.namespace,
//		"namespace",
//		"n",
//		"flux-system",
//		"diff the requested object(s) in this namespace",
//	)
//	diffCmd.PersistentFlags().BoolVarP(
//		&diffArgs.short,
//		"short",
//		"",
//		false,
//		"only print summary",
//	)
//	rootCmd.AddCommand(diffCmd)
//}
//
//func printDiff(base, now []*resource.Resource) error {
//	a, err := os.Create("a.yaml")
//	if err != nil {
//		return err
//	}
//	b, err := os.Create("b.yaml")
//	if err != nil {
//		return err
//	}
//
//	for _, res := range base {
//		a.WriteString(res.MustYaml() + "\n---\n")
//	}
//	for _, res := range now {
//		b.WriteString(res.MustYaml() + "\n---\n")
//	}
//
//	a.Close()
//	b.Close()
//
//	af, bf, err := ytbx.LoadFiles("a.yaml", "b.yaml")
//	if err != nil {
//		return err
//	}
//
//	report, err := dyff.CompareInputFiles(af, bf, dyff.IgnoreOrderChanges(true))
//	if err != nil {
//		return err
//	}
//
//	if diffArgs.short {
//		reporter := dyff.BriefReport{
//			Report: report,
//		}
//		return reporter.WriteReport(os.Stdout)
//	}
//
//	reporter := dyff.HumanReport{
//		Report:                report,
//		MultilineContextLines: 2,
//		NoTableStyle:          false,
//		DoNotInspectCerts:     false,
//		OmitHeader:            true,
//		UseGoPatchPaths:       false,
//	}
//
//	return reporter.WriteReport(os.Stdout)
//}
//
//func resourceEquals(a, b *resource.Resource) bool {
//	return a.GetKind() == b.GetKind() &&
//		a.GetApiVersion() == b.GetApiVersion() &&
//		a.GetName() == b.GetName() &&
//		a.GetNamespace() == b.GetNamespace()
//}
