// Package repository is the migration target for storage implementations.
//
// During wave 2, SQL-backed storage still lives in internal/app as compatibility
// shims. New persistence implementations should move here behind the interfaces
// consumed by internal/usecase.
package repository
