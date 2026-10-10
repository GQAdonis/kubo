package confirm

// hg1Disabled lists mechanisms whose HG-1 check failed on a real machine: a
// same-user background process was able to satisfy the prompt. A mechanism in
// this table is refused by its resolver. The evidence lives in
// MECHANISMS.md and evidence/. It is empty because no failing result has
// been recorded; the operator adds an entry (with the reason) on a failure.
var hg1Disabled = map[string]string{}

// disabledMechanism reports whether a mechanism is disabled and why.
func disabledMechanism(name string) (string, bool) {
	reason, off := hg1Disabled[name]
	return reason, off
}
