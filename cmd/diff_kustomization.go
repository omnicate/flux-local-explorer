package cmd

//
//import (
//	"fmt"
//
//	"github.com/spf13/cobra"
//	"golang.org/x/sync/errgroup"
//	"sigs.k8s.io/kustomize/api/resource"
//	"sigs.k8s.io/kustomize/kyaml/filesys"
//
//	"github.com/omnicate/flx/loader/manager"
//)
//
//// diffKustomizationCmd compares two kustomizations
//var diffKustomizationCmd = &cobra.Command{
//	Use:     "kustomization",
//	Aliases: []string{"ks"},
//	Short:   "Flux Kustomization resources (ks)",
//	PreRunE: getCmd.PreRunE,
//	RunE: func(cmd *cobra.Command, args []string) error {
//		if len(args) > 0 {
//			getArgs.name = args[0]
//		}
//
//		var baseRes []*resource.Resource
//		var res []*resource.Resource
//
//		var eg errgroup.Group
//		eg.Go(func() error {
//			// Base work tree (doesn't use local repo reference, will use "master" or whatever
//			// is specified in flux):
//			baseOpts := []manager.Option{
//				manager.WithLogger(logger),
//				manager.WithRepoCachePath(rootArgs.cacheDir),
//				manager.WithGitForceHTTPS(rootArgs.gitForceHTTPS),
//			}
//			logger.Info().Msg("Getting base")
//			baseLoader := manager.NewLoader(baseOpts...)
//			baseSeq, err := baseLoader.Load(
//				filesys.MakeFsOnDisk(),
//				rootArgs.fluxDir,
//				"flux-system",
//			)
//			if err != nil {
//				return err
//			}
//			baseResults, err := getResultsFromSeq(baseSeq.Kustomizations)
//			if err != nil {
//				return fmt.Errorf("getting base results: %w", err)
//			}
//			for _, r := range baseResults {
//				baseRes = append(baseRes, r.Resources...)
//			}
//			return nil
//		})
//
//		eg.Go(func() error {
//			// Modified tree:
//			logger.Info().Msg("Getting current version")
//			result, err := repoLoader.Load(
//				filesys.MakeFsOnDisk(),
//				rootArgs.fluxDir,
//				"flux-system",
//			)
//			if err != nil {
//				return err
//			}
//			results, err := getResultsFromSeq(result.Kustomizations)
//			if err != nil {
//				return fmt.Errorf("getting changed results: %w", err)
//			}
//			for _, r := range results {
//				res = append(res, r.Resources...)
//			}
//
//			return nil
//		})
//
//		if err := eg.Wait(); err != nil {
//			return err
//		}
//
//		return printDiff(baseRes, res)
//	},
//}
//
//func init() {
//	diffCmd.AddCommand(diffKustomizationCmd)
//}
