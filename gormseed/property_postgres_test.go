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

// JunctionWarehouse, JunctionProduct and JunctionInventoryItem mirror
// gormseed/megamart_test.go's MMInventoryItem shape: a composite primary
// key made of exactly its two required references' foreign key columns —
// a many-to-many join table in every way except that GORM sees it as a
// user-declared struct, not an auto-generated one. Scale is deliberately
// drawn small (1-4): a wide driver-parent-to-target-parent gap is exactly
// what triggers a primary key collision if child counts aren't capped at
// the target's own row count.
type JunctionWarehouse struct {
	ID uint `gorm:"primaryKey"`
}
type JunctionProduct struct {
	ID uint `gorm:"primaryKey"`
}
type JunctionInventoryItem struct {
	WarehouseID    uint `gorm:"primaryKey"`
	Warehouse      JunctionWarehouse
	ProductID      uint `gorm:"primaryKey"`
	Product        JunctionProduct
	QuantityOnHand int
}

func TestSeed_Postgres_Property_JunctionNeverDuplicatesAPrimaryKey(t *testing.T) {
	db := postgresDB(t)
	models := []any{&JunctionWarehouse{}, &JunctionProduct{}, &JunctionInventoryItem{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Uint64().Draw(rt, "seed")
		scale := rapid.IntRange(1, 4).Draw(rt, "scale")

		truncateAll(t, db, "junction_inventory_items", "junction_products", "junction_warehouses")
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(seed), autoseed.WithScale(scale)); err != nil {
			rt.Fatalf("Seed(seed=%d, scale=%d): %v", seed, scale, err)
		}

		assertNoOrphans(rt, db, "junction_inventory_items", "warehouse_id", "junction_warehouses", "id")
		assertNoOrphans(rt, db, "junction_inventory_items", "product_id", "junction_products", "id")

		var total, distinct int64
		db.Table("junction_inventory_items").Count(&total)
		db.Raw(`SELECT count(*) FROM (SELECT DISTINCT warehouse_id, product_id FROM junction_inventory_items) d`).Scan(&distinct)
		if total != distinct {
			rt.Fatalf("seed=%d scale=%d: %d rows but only %d distinct (warehouse_id, product_id) pairs", seed, scale, total, distinct)
		}
	})
}

// SharedKeyProduct and SharedKeyProfile mirror the GORM equivalent of EF
// Core's table splitting: ProfileProductID is both its own primary key
// and its only foreign key, so a driver row's key gets copied verbatim
// into the child's own primary key. Two children for the same product
// would collide on it — exactly what an uncapped long-tail draw risks.
type SharedKeyProduct struct {
	ID uint `gorm:"primaryKey"`
}
type SharedKeyProfile struct {
	ProfileProductID uint             `gorm:"primaryKey"`
	Product          SharedKeyProduct `gorm:"foreignKey:ProfileProductID"`
}

func TestSeed_Postgres_Property_SharedPrimaryKeyNeverDuplicates(t *testing.T) {
	db := postgresDB(t)
	models := []any{&SharedKeyProduct{}, &SharedKeyProfile{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Uint64().Draw(rt, "seed")
		scale := rapid.IntRange(1, 15).Draw(rt, "scale")

		truncateAll(t, db, "shared_key_profiles", "shared_key_products")
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(seed), autoseed.WithScale(scale)); err != nil {
			rt.Fatalf("Seed(seed=%d, scale=%d): %v", seed, scale, err)
		}

		assertNoOrphans(rt, db, "shared_key_profiles", "profile_product_id", "shared_key_products", "id")

		var total, distinct int64
		db.Table("shared_key_profiles").Count(&total)
		db.Raw(`SELECT count(DISTINCT profile_product_id) FROM shared_key_profiles`).Scan(&distinct)
		if total != distinct {
			rt.Fatalf("seed=%d scale=%d: %d rows but only %d distinct profile_product_id values", seed, scale, total, distinct)
		}
	})
}

// SelfJunctionPerson mirrors a self-referencing many-to-many association
// (a "follows" or "friends" table) — GORM's auto-generated join table has
// two required references that both target Person, told apart only by
// their own foreign key columns (FollowingID vs FollowerID), never by
// Target name.
type SelfJunctionPerson struct {
	ID        uint                  `gorm:"primaryKey"`
	Followers []*SelfJunctionPerson `gorm:"many2many:self_junction_follows;joinForeignKey:FollowingID;joinReferences:FollowerID"`
}

func TestSeed_Postgres_Property_SelfReferencingJunctionNeverSelfLoopsOrDuplicates(t *testing.T) {
	db := postgresDB(t)
	models := []any{&SelfJunctionPerson{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Uint64().Draw(rt, "seed")
		scale := rapid.IntRange(2, 6).Draw(rt, "scale") // scale=1: only one Person exists, a self-loop would be the only possible pairing

		truncateAll(t, db, "self_junction_follows", "self_junction_people")
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(seed), autoseed.WithScale(scale)); err != nil {
			rt.Fatalf("Seed(seed=%d, scale=%d): %v", seed, scale, err)
		}

		assertNoOrphans(rt, db, "self_junction_follows", "following_id", "self_junction_people", "id")
		assertNoOrphans(rt, db, "self_junction_follows", "follower_id", "self_junction_people", "id")

		var total, distinct, selfLoops int64
		db.Table("self_junction_follows").Count(&total)
		db.Raw(`SELECT count(*) FROM (SELECT DISTINCT following_id, follower_id FROM self_junction_follows) d`).Scan(&distinct)
		db.Raw(`SELECT count(*) FROM self_junction_follows WHERE following_id = follower_id`).Scan(&selfLoops)

		if total != distinct {
			rt.Fatalf("seed=%d scale=%d: %d rows but only %d distinct (following_id, follower_id) pairs", seed, scale, total, distinct)
		}
		if selfLoops != 0 {
			rt.Fatalf("seed=%d scale=%d: %d rows follow themselves (following_id = follower_id)", seed, scale, selfLoops)
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
