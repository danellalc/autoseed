package gormseed_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// ChainRoot, ChainMiddle and ChainLeaf mirror the .NET sibling's
// LinearRequiredChainContext fixture: a straight line of required
// references, verified for referential integrity under a rapid-varied
// seed and scale.
type ChainRoot struct {
	ID uint `gorm:"primaryKey"`
}

type ChainMiddle struct {
	ID          uint `gorm:"primaryKey"`
	ChainRootID uint `gorm:"not null"`
	ChainRoot   ChainRoot
}

type ChainLeaf struct {
	ID            uint `gorm:"primaryKey"`
	ChainMiddleID uint `gorm:"not null"`
	ChainMiddle   ChainMiddle
}

func TestSeed_Postgres_Property_LinearChain(t *testing.T) {
	db := postgresDB(t)
	models := []any{&ChainRoot{}, &ChainMiddle{}, &ChainLeaf{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Uint64().Draw(rt, "seed")
		scale := rapid.IntRange(1, 15).Draw(rt, "scale")

		truncateAll(t, db, "chain_leafs", "chain_middles", "chain_roots")
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(seed), autoseed.WithScale(scale)); err != nil {
			rt.Fatalf("Seed(seed=%d, scale=%d): %v", seed, scale, err)
		}

		assertNoOrphans(rt, db, "chain_middles", "chain_root_id", "chain_roots", "id")
		assertNoOrphans(rt, db, "chain_leafs", "chain_middle_id", "chain_middles", "id")
	})
}

// DiamondRoot, DiamondLeft, DiamondRight and DiamondMerge mirror the .NET
// sibling's DiamondRequiredContext: two independent required principals
// merging into one dependent, the classic case a naive single-driver
// algorithm could get wrong.
type DiamondRoot struct {
	ID uint `gorm:"primaryKey"`
}

type DiamondLeft struct {
	ID            uint `gorm:"primaryKey"`
	DiamondRootID uint `gorm:"not null"`
	DiamondRoot   DiamondRoot
}

type DiamondRight struct {
	ID            uint `gorm:"primaryKey"`
	DiamondRootID uint `gorm:"not null"`
	DiamondRoot   DiamondRoot
}

type DiamondMerge struct {
	ID             uint `gorm:"primaryKey"`
	DiamondLeftID  uint `gorm:"not null"`
	DiamondLeft    DiamondLeft
	DiamondRightID uint         `gorm:"not null"`
	DiamondRight   DiamondRight `gorm:"foreignKey:DiamondRightID"`
}

func TestSeed_Postgres_Property_Diamond(t *testing.T) {
	db := postgresDB(t)
	models := []any{&DiamondRoot{}, &DiamondLeft{}, &DiamondRight{}, &DiamondMerge{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Uint64().Draw(rt, "seed")
		scale := rapid.IntRange(1, 15).Draw(rt, "scale")

		truncateAll(t, db, "diamond_merges", "diamond_lefts", "diamond_rights", "diamond_roots")
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(seed), autoseed.WithScale(scale)); err != nil {
			rt.Fatalf("Seed(seed=%d, scale=%d): %v", seed, scale, err)
		}

		assertNoOrphans(rt, db, "diamond_lefts", "diamond_root_id", "diamond_roots", "id")
		assertNoOrphans(rt, db, "diamond_rights", "diamond_root_id", "diamond_roots", "id")
		assertNoOrphans(rt, db, "diamond_merges", "diamond_left_id", "diamond_lefts", "id")
		assertNoOrphans(rt, db, "diamond_merges", "diamond_right_id", "diamond_rights", "id")
	})
}

// OrgNode mirrors the .NET sibling's NullableSelfReferenceContext: every
// row's parent, when set, must be a real, different row.
type OrgNode struct {
	ID       uint `gorm:"primaryKey"`
	ParentID *uint
	Parent   *OrgNode
}

func TestSeed_Postgres_Property_NullableSelfReference(t *testing.T) {
	db := postgresDB(t)
	models := []any{&OrgNode{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Uint64().Draw(rt, "seed")
		scale := rapid.IntRange(1, 15).Draw(rt, "scale")

		truncateAll(t, db, "org_nodes")
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(seed), autoseed.WithScale(scale)); err != nil {
			rt.Fatalf("Seed(seed=%d, scale=%d): %v", seed, scale, err)
		}

		assertNoOrphansNullable(rt, db, "org_nodes", "parent_id", "org_nodes", "id")

		var nodes []OrgNode
		db.Find(&nodes)
		for _, node := range nodes {
			if node.ParentID != nil && *node.ParentID == node.ID {
				rt.Fatalf("seed=%d scale=%d: node %d is its own parent", seed, scale, node.ID)
			}
		}
	})
}

func truncateAll(t *testing.T, db *gorm.DB, tables ...string) {
	t.Helper()
	stmt := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("truncate %v: %v", tables, err)
	}
}

// failer is satisfied by both *testing.T and *rapid.T, so the orphan
// assertions below serve both the plain integration tests and the
// rapid-driven property tests without duplicating the SQL.
type failer interface {
	Helper()
	Fatalf(format string, args ...any)
}

func assertNoOrphans(rt failer, db *gorm.DB, childTable, childColumn, parentTable, parentColumn string) {
	rt.Helper()
	query := fmt.Sprintf(
		`SELECT count(*) FROM %s c LEFT JOIN %s p ON c.%s = p.%s WHERE p.%s IS NULL`,
		childTable, parentTable, childColumn, parentColumn, parentColumn,
	)
	var orphanCount int64
	if err := db.Raw(query).Scan(&orphanCount).Error; err != nil {
		rt.Fatalf("orphan check %s.%s -> %s.%s: %v", childTable, childColumn, parentTable, parentColumn, err)
	}
	if orphanCount != 0 {
		rt.Fatalf("%d rows in %s reference a non-existent %s via %s", orphanCount, childTable, parentTable, childColumn)
	}
}

func assertNoOrphansNullable(rt failer, db *gorm.DB, childTable, childColumn, parentTable, parentColumn string) {
	rt.Helper()
	query := fmt.Sprintf(
		`SELECT count(*) FROM %s c LEFT JOIN %s p ON c.%s = p.%s WHERE c.%s IS NOT NULL AND p.%s IS NULL`,
		childTable, parentTable, childColumn, parentColumn, childColumn, parentColumn,
	)
	var orphanCount int64
	if err := db.Raw(query).Scan(&orphanCount).Error; err != nil {
		rt.Fatalf("orphan check %s.%s -> %s.%s: %v", childTable, childColumn, parentTable, parentColumn, err)
	}
	if orphanCount != 0 {
		rt.Fatalf("%d rows in %s reference a non-existent %s via %s", orphanCount, childTable, parentTable, childColumn)
	}
}
