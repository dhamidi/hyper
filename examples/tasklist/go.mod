module github.com/dhamidi/hyper/examples/tasklist

go 1.26.1

require (
	github.com/dhamidi/htmlc v0.0.0
	github.com/dhamidi/hyper v0.0.0
)

replace (
	github.com/dhamidi/htmlc => ../../htmlc
	github.com/dhamidi/hyper => ../..
)
