# stdlib

[![Go Reference](https://pkg.go.dev/badge/github.com/abemedia/stdlib.svg)](https://pkg.go.dev/github.com/abemedia/stdlib)
[![CI](https://github.com/abemedia/stdlib/actions/workflows/test.yml/badge.svg)](https://github.com/abemedia/stdlib/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/abemedia/stdlib)](https://goreportcard.com/report/github.com/abemedia/stdlib)

A static analysis tool for Go that detects usage of functions from third-party libraries and
suggests replacements using standard library functions or built-in language features. It leverages
Go’s [`analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis) package to provide automated
refactorings and improvement suggestions.

## Usage

```bash
# Install
go install github.com/abemedia/stdlib/cmd/stdlib@latest

# Use
stdlib ./...
```

## Replacements

See below for all the replacements of packages, functions and types. They will only be replaced if
the Go version of the file supports the new package.

### Packages

Replaces the imports of packages from `golang.org/x` which now exist in the stdlib.

| Before                      | After          |
| --------------------------- | -------------- |
| `golang.org/x/exp/maps`     | `maps`         |
| `golang.org/x/exp/rand`     | `math/rand/v2` |
| `golang.org/x/exp/slices`   | `slices`       |
| `golang.org/x/exp/slog`     | `log/slog`     |
| `golang.org/x/net/context`  | `context`      |
| `golang.org/x/sync/syncmap` | `sync`         |

### Functions

Expand the sections below to see the supported replacements for each package.

<details>
<summary>github.com/samber/lo</summary>

#### `Chunk`

**Before:**

```go
result := lo.Chunk(slice, size)
```

**After:**

```go
result := slices.Chunk(slice, size)
```

#### `Drop`

**Before:**

```go
a := []int{0, 1, 2, 3, 4, 5}
b := lo.Drop(a, 2)
```

**After:**

```go
a := []int{0, 1, 2, 3, 4, 5}
b := a[2:]
```

#### `DropRight`

**Before:**

```go
a := []int{0, 1, 2, 3, 4, 5}
b := lo.DropRight(a, 2)
```

**After:**

```go
a := []int{0, 1, 2, 3, 4, 5}
b := a[:len(a)-2]
```

#### `Contains`

**Before:**

```go
if lo.Contains(slice, target) {
    // do something
}
```

**After:**

```go
if slices.Contains(slice, target) {
    // do something
}
```

#### `ContainsBy`

**Before:**

```go
if lo.ContainsBy(slice, func(item int) bool {
    return item > 10
}) {
    // do something
}
```

**After:**

```go
if slices.ContainsFunc(slice, func(item int) bool {
    return item > 10
}) {
    // do something
}
```

#### `IndexOf`

**Before:**

```go
idx := lo.IndexOf(slice, target)
```

**After:**

```go
idx := slices.Index(slice, target)
```

#### `Min`

**Before:**

```go
min := lo.Min(slice)
```

**After:**

```go
min := slices.Min(slice)
```

#### `MinBy`

**Before:**

```go
min := lo.MinBy(slice, func(a, b int) bool {
    return a < b
})
```

**After:**

```go
min := slices.MinFunc(slice, func(a, b int) int {
    return cmp.Compare(a, b)
})
```

#### `Max`

**Before:**

```go
max := lo.Max(slice)
```

**After:**

```go
max := slices.Max(slice)
```

#### `MaxBy`

**Before:**

```go
max := lo.MaxBy(slice, func(a, b int) bool {
    return a > b
})
```

**After:**

```go
max := slices.MaxFunc(slice, func(a, b int) int {
    return cmp.Compare(a, b)
})
```

#### `IsSorted`

**Before:**

```go
if lo.IsSorted(slice) {
    // do something
}
```

**After:**

```go
if slices.IsSorted(slice) {
    // do something
}
```

#### `IsSortedByKey`

**Before:**

```go
sorted := lo.IsSortedByKey(slice, func(a string) string {
    return a
})
```

**After:**

```go
sorted := slices.IsSortedFunc(slice, func(a, next string) int {
    return cmp.Compare(a, next)
})
```

#### `Flatten`

**Before:**

```go
flattened := lo.Flatten(sliceOfSlices)
```

**After:**

```go
flattened := slices.Concat(sliceOfSlices...)
```

#### `Keys`

**Before:**

```go
keys := lo.Keys(m)
```

**After:**

```go
keys := maps.Keys(m)
```

#### `Values`

**Before:**

```go
values := lo.Values(m)
```

**After:**

```go
values := maps.Values(m)
```

#### `CoalesceOrEmpty`

**Before:**

```go
result := lo.CoalesceOrEmpty(s1, s2, s3)
```

**After:**

```go
result := cmp.Or(s1, s2, s3)
```

#### `RuneLength`

**Before:**

```go
n := lo.RuneLength(s)
```

**After:**

```go
n := utf8.RuneCountInString(s)
```

</details>

<details>
<summary>github.com/samber/lo/mutable</summary>

#### `Reverse`

**Before:**

```go
lo.Reverse(slice)
```

**After:**

```go
slices.Reverse(slice)
```

</details>
