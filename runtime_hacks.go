package ptrguard

import _ "unsafe" // enable go:linkname

type _dbgVar struct {
	name  string
	value *int32
}

//go:linkname _dbgvars runtime.dbgvars
var _dbgvars []_dbgVar

var cgocheck = func() *int32 {
	for i := range _dbgvars {
		if _dbgvars[i].name == "cgocheck" {
			return _dbgvars[i].value
		}
	}
	panic("Couln't find cgocheck debug variable")
}()
