package dissect

// Manual operator-role additions that genops.go hasn't picked up yet.
//
// Reasons we might need entries here:
//   - A new operator shipped in a season newer than our last genops run.
//   - Ubisoft's public operator list is out of sync with an in-game build
//     (e.g. alpha/beta ops not yet on the website, such as a Y11S1_Alpha
//     replay recorded against pre-release assets).
//   - The operator's Ubisoft slug doesn't match our const's Go name, so
//     the generator silently skipped it.
//
// Entries here are merged into _operatorRoles at package init time, so
// Role() resolves them exactly like generated entries. Re-running
// `go generate` will NOT wipe this file — it only writes
// operator_roles.go. Once an operator appears in Ubisoft's public data
// AND has a matching Go const in header.go, prefer moving the entry to
// the generated file via genops and deleting it from here.

func init() {
	// Y11S1_Alpha03 attacker. Confirmed as Attack because the single
	// replay that surfaced this ID had the carrying player on the
	// attacking team (teamIndex matched the StartingScore-derived Attack
	// side). The replay header attached a Gridlock role-portrait, which
	// suggests pre-release placeholder cosmetics rather than a bona-fide
	// Gridlock — the operator ID is new (higher than any Y10 op) and no
	// existing const maps to it.
	//
	// TODO: once this operator has an official name, promote it to a
	// named const in header.go and let genops.go regenerate the role
	// map.
	_operatorRoles[444310693746] = Attack
}
