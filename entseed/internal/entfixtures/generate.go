// Package entfixtures holds a small, real ent schema and its generated
// client, used only by entseed's own tests to exercise the adapter
// against real ent code (ent has no runtime schema introspection the
// way GORM does — a genuine client must be generated to test against).
package entfixtures

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema
