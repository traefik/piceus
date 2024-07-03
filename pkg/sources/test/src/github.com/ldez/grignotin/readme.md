# Grignotin

A collection of small helpers around Go proxy, Go meta information, etc.

## goproxy

A small Go proxy client to get information about a module from a Go proxy.

```go
package main

import (
	"fmt"

	"github.com/ldez/grignotin/goproxy"
)

func main() {
	client := goproxy.NewClient("")

	versions, err := client.GetVersions("github.com/ldez/grignotin")
	if err != nil {
		panic(err)
	}

	fmt.Println(versions)
}
```

## metago

A small lib to fetch meta information (`go-import`, `go-source`) for a module.

```go
package main

import (
	"fmt"

	"github.com/ldez/grignotin/metago"
)

func main() {
	meta, err := metago.Get("github.com/ldez/grignotin")
	if err != nil {
		panic(err)
	}

	fmt.Println(meta)
}
```
