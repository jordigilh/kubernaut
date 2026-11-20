package routing

// ParseTargetResourceForTest exposes parseTargetResource for external test
// packages (#2658): the suffix-aware parser is the single seam the gates and
// the DataStorage query key share.
var ParseTargetResourceForTest = parseTargetResource
