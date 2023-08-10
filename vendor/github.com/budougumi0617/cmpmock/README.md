# cmpmock

[![Go Reference](https://pkg.go.dev/badge/github.com/budougumi0617/cmpmock.svg)](https://pkg.go.dev/github.com/budougumi0617/cmpmock)
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![test](https://github.com/budougumi0617/cmpmock/workflows/test/badge.svg)](https://github.com/budougumi0617/cmpmock/actions?query=workflow%3Atest)
[![reviewdog](https://github.com/budougumi0617/cmpmock/workflows/reviewdog/badge.svg)](https://github.com/budougumi0617/cmpmock/actions?query=workflow%3Areviewdog)

Readable & Flexible matcher for https://github.com/golang/mock

## Description
cmpmock provides a simple custom matcher. it is be able to modify behavior with `github.com/google/go-cmp/cmp/cmpopts`.

```go
import "github.com/google/go-cmp/cmp"

func DiffEq(v interface{}, opts ...cmp.Option) gomock.Matcher
```

If `DiffEq` is set no `opts`, default behavior ignores a time differences of less than a second.


### Readable ouput

Default output
```
expected call at /Users/budougumi0617/go/src/github.com/budougumi0617/cmpmock/_example/repo_test.go:26 doesn't match the argument at index 1.
Got: &{John Due Tokyo 2021-04-23 02:46:58.145696 +0900 JST m=+0.000595005}
Want: is equal to &{John Due Tokyo 2021-04-23 02:46:48.145646 +0900 JST m=-9.999455563}
```

use `cmpmock.DiffEq`
```
expected call at /Users/budougumi0617/go/src/github.com/budougumi0617/cmpmock/_example/repo_test.go:27 doesn't match the argument at index 1.
Got: &{John Due Tokyo 2021-04-23 02:46:33.290458 +0900 JST m=+0.001035665}
Want: diff(-got +want) is   &_example.User{
 	Name:     "John Due",
 	Address:  "Tokyo",
- 	CreateAt: s"2021-04-23 02:46:33.290458 +0900 JST m=+0.001035665",
+ 	CreateAt: s"2021-04-23 02:46:23.290383 +0900 JST m=-9.999039004",
}
```

## Usage

```go
type UserRepo interface {
  Save(context.Context, *User) error
}

wantUser := &User{}
mrepo := mock.NewMockUserRepo(ctrl)
mrepo.EXPECT().Save(ctx, cmpmock.DiffEq(wantUser)).Return(nil)
```

## Installation

```bash
$ go get -u github.com/budougumi0617/cmpmock
```

## License

[MIT](./LICENSE)

## Author
Yocihiro Shimizu(@budougumi0617)