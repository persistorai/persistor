// Package harness holds the Claude Code install artifacts (hooks, the
// /consolidate skill, the settings snippet) that wire the memory
// engine into a live session. Nothing here runs automatically — installation is
// the deliberate cutover step. The accompanying test exercises the hooks
// mechanically so they keep emitting/enqueuing the right shape.
package harness
