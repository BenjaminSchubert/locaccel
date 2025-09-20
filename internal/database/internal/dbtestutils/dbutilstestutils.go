package dbtestutils

//go:generate go tool github.com/tinylib/msgp -io=false -tests=false
type TestObj struct {
	Value string
}
