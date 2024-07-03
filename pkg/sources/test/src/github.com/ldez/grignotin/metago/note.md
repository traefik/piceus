Vanity imports:

- https://sagikazarmark.hu/blog/vanity-import-paths-in-go/
- https://github.com/golang/tools/tree/0a99049195aff55f007fc4dfd48e3ec2b4d5f602/go/vcs
    ```
    import "golang.org/x/tools/go/vcs"
  
    repoRoot, err := vcs.RepoRootForImportDynamic("pkname", false)
    ```
- `go1.14/src/cmd/go/internal/get`