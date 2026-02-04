// Package engine ties the workflow resolver and scheduler together. It exposes
// a persistence-backed engine that can start new workflow runs, resume
// existing ones, and incrementally update scheduler decisions as modules
// complete or fail.
package engine
