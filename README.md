[![Go Report Card](https://goreportcard.com/badge/github.com/lcd1232/dqueue)](https://goreportcard.com/report/github.com/lcd1232/paddle)
[![Build Status](https://img.shields.io/github/workflow/status/lcd1232/dqueue/test)](https://github.com/lcd1232/dqueue/actions)
[![Coverage](https://img.shields.io/codecov/c/github/lcd1232/dqueue)](https://codecov.io/gh/lcd1232/dqueue)
[![Godoc](http://img.shields.io/badge/go-documentation-blue.svg)](https://pkg.go.dev/github.com/lcd1232/dqueue)
[![Releases](https://img.shields.io/github/v/tag/lcd1232/dqueue.svg)](https://github.com/lcd1232/dqueue/releases)
[![LICENSE](https://img.shields.io/github/license/lcd1232/dqueue.svg)](https://github.com/lcd1232/dqueue/blob/master/LICENSE)
# dqueue
> Queue with delayed items

## Installation

```shell
go get github.com/lcd1232/dqueue
```

## Usage

```go
package main
import (
    "context"
    "fmt"
    "time"

    "github.com/lcd1232/dqueue"
)

func main() {
    q := dqueue.NewQueue()
    defer q.Stop(context.Background())
    // Insert "some value" with delay 1 hour 
    q.Insert("some value", time.Hour)
    q.Insert(2, time.Minute)
    v, ok := q.Pop()
    fmt.Println(v, ok) // Prints <nil> false
    // Due to golang timer implementation it can take a little more time 
    time.Sleep(time.Minute+time.Second) 
    v, ok = q.Pop()
    fmt.Println(v, ok) // Prints 2 true

    // Use context cancellation
    ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
    defer cancel()
    v, ok = q.PopCtx(ctx)
    fmt.Println(v, ok) // Prints <nil> false
    
    // Get value from channel
    select {
    case v = <- q.Channel():
    fmt.Println(v, ok) // Prints some value false
    }
}

```

## Project versioning

dqueue uses [semantic versioning](http://semver.org).
API should not change between patch and minor releases.
New minor versions may add additional features to the API.

## Licensing

"The code in this project is licensed under MIT license."
