//go:build !freebsd
// +build !freebsd

package signature

// BuiltinDefaultPolicyPath is the policy path used for DefaultPolicy().
// DO NOT change this, instead see systemDefaultPolicyPath above.
const BuiltinDefaultPolicyPath = "/etc/containers/policy.json"
