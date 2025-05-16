package dbtestutils

//go:generate msgp -io=false -tests=false
type TestObj struct {
	Value string
}
