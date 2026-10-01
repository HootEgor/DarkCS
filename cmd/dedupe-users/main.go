// Command dedupe-users finds user documents that share an identifier (phone, email,
// telegram_id, instagram_id, uuid) and merges each group into one document. It must run
// before the unique user indexes can be created (see repository.EnsureIndexes).
//
// Duplicates come from the old lookup bug, where an unnormalized phone missed the stored
// "+digits" form and a second user was registered, and from users saved with the "+"
// placeholder phone.
//
// It is a dry run by default and only prints the plan. With -apply it first writes every
// affected document to a backup file (MongoDB extended JSON, one per line), then for each
// group: deletes the duplicates, updates the kept document, and re-points baskets.
//
// Usage:
//
//	go run ./cmd/dedupe-users -conf config.yml           # dry run
//	go run ./cmd/dedupe-users -conf config.yml -apply    # merge
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"DarkCS/entity"
	"DarkCS/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	usersCollection  = "users"
	basketCollection = "baskets"
)

// roleRank orders roles so a merge never downgrades someone's access.
var roleRank = map[string]int{entity.AdminRole: 4, entity.ManagerRole: 3, entity.UserRole: 2, entity.GuestRole: 1}

// mergeableStrings are filled on the kept document from duplicates when empty there.
var mergeableStrings = []string{
	"uuid", "name", "email", "phone", "address", "telegram_username", "instagram_id",
	"instagram_username", "smart_sender_id", "zoho_id",
}

func main() {
	confPath := flag.String("conf", "config.yml", "path to config file")
	apply := flag.Bool("apply", false, "apply the merge (default is a dry run)")
	backupPath := flag.String("backup", fmt.Sprintf("dedupe-users-backup-%s.jsonl", time.Now().Format("20060102-150405")), "backup file written before applying")
	flag.Parse()

	conf := config.MustLoad(*confPath)
	ctx := context.Background()

	opts := options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%s", conf.Mongo.Host, conf.Mongo.Port))
	if conf.Mongo.User != "" {
		opts.SetAuth(options.Credential{Username: conf.Mongo.User, Password: conf.Mongo.Password, AuthSource: conf.Mongo.Database})
	}
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer client.Disconnect(ctx)
	db := client.Database(conf.Mongo.Database)
	users := db.Collection(usersCollection)

	var docs []bson.M
	cur, err := users.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"_id", 1}}))
	if err != nil {
		log.Fatalf("find users: %v", err)
	}
	if err = cur.All(ctx, &docs); err != nil {
		log.Fatalf("decode users: %v", err)
	}
	fmt.Printf("loaded %d users\n", len(docs))

	// Phones are compared in normalized form; fixes are applied to non-duplicates too.
	phoneFixes := 0
	for _, d := range docs {
		if p, ok := d["phone"].(string); ok && entity.NormalizePhone(p) != p {
			phoneFixes++
		}
	}

	groups := groupDuplicates(docs)
	fmt.Printf("duplicate groups: %d, phones to normalize: %d\n\n", len(groups), phoneFixes)

	for i, g := range groups {
		keep, merged := mergeGroup(g)
		fmt.Printf("group %d: keep %s (%s)\n", i+1, idOf(keep), describe(keep))
		for _, d := range g {
			if d["_id"] != keep["_id"] {
				fmt.Printf("    drop %s (%s)\n", idOf(d), describe(d))
			}
		}
		if len(merged) > 0 {
			fmt.Printf("    set on kept: %v\n", merged)
		}
	}

	if !*apply {
		fmt.Println("\ndry run: nothing changed. Re-run with -apply to merge.")
		return
	}

	if err = writeBackup(*backupPath, docs); err != nil {
		log.Fatalf("backup: %v", err)
	}
	fmt.Printf("\nbackup written to %s\n", *backupPath)

	baskets := db.Collection(basketCollection)
	for i, g := range groups {
		if err = applyGroup(ctx, users, baskets, g); err != nil {
			log.Fatalf("group %d: %v (earlier groups are already applied; backup is in %s)", i+1, err, *backupPath)
		}
	}

	// Normalize remaining phones (including "+" placeholders, which become "").
	fixed := 0
	cur, err = users.Find(ctx, bson.D{{"phone", bson.D{{"$type", "string"}}}})
	if err != nil {
		log.Fatalf("find phones: %v", err)
	}
	for cur.Next(ctx) {
		var d bson.M
		if err = cur.Decode(&d); err != nil {
			log.Fatalf("decode: %v", err)
		}
		p := d["phone"].(string)
		if n := entity.NormalizePhone(p); n != p {
			if _, err = users.UpdateByID(ctx, d["_id"], bson.M{"$set": bson.M{"phone": n}}); err != nil {
				log.Fatalf("normalize phone %s: %v", idOf(d), err)
			}
			fixed++
		}
	}
	if err = cur.Err(); err != nil {
		log.Fatalf("cursor: %v", err)
	}

	fmt.Printf("merged %d groups, normalized %d phones. Restart the service to create unique indexes.\n", len(groups), fixed)
}

// identifiers returns the identity keys of a document, e.g. "phone:+380...".
func identifiers(d bson.M) []string {
	var ids []string
	if p, ok := d["phone"].(string); ok {
		if n := entity.NormalizePhone(p); n != "" {
			ids = append(ids, "phone:"+n)
		}
	}
	for _, f := range []string{"uuid", "email", "instagram_id"} {
		if v, ok := d[f].(string); ok && v != "" {
			ids = append(ids, f+":"+strings.ToLower(v))
		}
	}
	if tg := toInt64(d["telegram_id"]); tg > 0 {
		ids = append(ids, fmt.Sprintf("telegram_id:%d", tg))
	}
	return ids
}

// groupDuplicates links documents sharing any identifier (transitively, via union-find)
// and returns the groups with more than one document.
func groupDuplicates(docs []bson.M) [][]bson.M {
	parent := make([]int, len(docs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}

	owner := map[string]int{}
	for i, d := range docs {
		for _, id := range identifiers(d) {
			if j, ok := owner[id]; ok {
				parent[find(i)] = find(j)
			} else {
				owner[id] = i
			}
		}
	}

	byRoot := map[int][]bson.M{}
	for i, d := range docs {
		r := find(i)
		byRoot[r] = append(byRoot[r], d)
	}
	var groups [][]bson.M
	for _, g := range byRoot {
		if len(g) > 1 {
			groups = append(groups, g)
		}
	}
	sort.Slice(groups, func(a, b int) bool { return idOf(groups[a][0]) < idOf(groups[b][0]) })
	return groups
}

// mergeGroup picks the document to keep and returns the fields to set on it.
// Kept: highest role, then has a name, then has a Zoho ID, then oldest.
// Empty fields are filled from the duplicates (oldest first); promo expiry takes the
// latest; conversation takes the longest; blocked is kept if any duplicate is blocked.
func mergeGroup(g []bson.M) (bson.M, bson.M) {
	sorted := append([]bson.M(nil), g...)
	sort.SliceStable(sorted, func(a, b int) bool {
		x, y := sorted[a], sorted[b]
		if rx, ry := roleRank[str(x, "role")], roleRank[str(y, "role")]; rx != ry {
			return rx > ry
		}
		if nx, ny := str(x, "name") != "", str(y, "name") != ""; nx != ny {
			return nx
		}
		if zx, zy := str(x, "zoho_id") != "", str(y, "zoho_id") != ""; zx != zy {
			return zx
		}
		return idOf(x) < idOf(y)
	})
	keep := sorted[0]
	others := sorted[1:]
	sort.SliceStable(others, func(a, b int) bool { return idOf(others[a]) < idOf(others[b]) })

	set := bson.M{}
	for _, f := range mergeableStrings {
		if str(keep, f) != "" {
			continue
		}
		for _, o := range others {
			if v := str(o, f); v != "" {
				set[f] = v
				break
			}
		}
	}
	// Store the resulting phone normalized; a "+" placeholder becomes "".
	phone := str(keep, "phone")
	if p, ok := set["phone"].(string); ok {
		phone = p
	}
	if n := entity.NormalizePhone(phone); n != str(keep, "phone") {
		set["phone"] = n
	}
	if toInt64(keep["telegram_id"]) <= 0 {
		for _, o := range others {
			if tg := toInt64(o["telegram_id"]); tg > 0 {
				set["telegram_id"] = tg
				break
			}
		}
	}

	latestPromo := dateOf(keep["promoExpire"])
	longestConv := arrLen(keep["conversation"])
	for _, o := range others {
		if t := dateOf(o["promoExpire"]); t.After(latestPromo) {
			latestPromo = t
			set["promoExpire"] = t
		}
		if n := arrLen(o["conversation"]); n > longestConv {
			longestConv = n
			set["conversation"] = o["conversation"]
		}
		if b, _ := o["blocked"].(bool); b {
			set["blocked"] = true
		}
	}
	return keep, set
}

func applyGroup(ctx context.Context, users, baskets *mongo.Collection, g []bson.M) error {
	keep, set := mergeGroup(g)
	keepUUID := str(keep, "uuid")
	if v, ok := set["uuid"].(string); ok {
		keepUUID = v
	}

	var dropIDs []any
	var dropUUIDs []string
	for _, d := range g {
		if d["_id"] == keep["_id"] {
			continue
		}
		dropIDs = append(dropIDs, d["_id"])
		if u := str(d, "uuid"); u != "" && u != keepUUID {
			dropUUIDs = append(dropUUIDs, u)
		}
	}

	// Delete first so the kept document can take over identifiers without conflicts.
	if _, err := users.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": dropIDs}}); err != nil {
		return fmt.Errorf("delete duplicates: %w", err)
	}
	if len(set) > 0 {
		if _, err := users.UpdateByID(ctx, keep["_id"], bson.M{"$set": set}); err != nil {
			return fmt.Errorf("update kept user: %w", err)
		}
	}

	// Baskets are keyed by user UUID: hand one over if the kept user has none, drop the rest.
	if keepUUID == "" || len(dropUUIDs) == 0 {
		return nil
	}
	n, err := baskets.CountDocuments(ctx, bson.M{"userUUID": keepUUID})
	if err != nil {
		return fmt.Errorf("count baskets: %w", err)
	}
	for _, u := range dropUUIDs {
		if n == 0 {
			res, err := baskets.UpdateOne(ctx, bson.M{"userUUID": u}, bson.M{"$set": bson.M{"userUUID": keepUUID, "user_uuid": keepUUID}})
			if err != nil {
				return fmt.Errorf("move basket: %w", err)
			}
			n = res.ModifiedCount
			continue
		}
		if _, err := baskets.DeleteMany(ctx, bson.M{"userUUID": u}); err != nil {
			return fmt.Errorf("delete basket: %w", err)
		}
	}
	return nil
}

// writeBackup saves every user, not only grouped ones, since phone normalization
// touches documents outside the groups too.
func writeBackup(path string, all []bson.M) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, d := range all {
		b, err := bson.MarshalExtJSON(d, true, false)
		if err != nil {
			return err
		}
		if _, err = f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return f.Sync()
}

func describe(d bson.M) string {
	return fmt.Sprintf("uuid=%s role=%s name=%q phone=%s tg=%d ig=%s zoho=%s",
		str(d, "uuid"), str(d, "role"), str(d, "name"), str(d, "phone"),
		toInt64(d["telegram_id"]), str(d, "instagram_id"), str(d, "zoho_id"))
}

func idOf(d bson.M) string {
	if id, ok := d["_id"].(primitive.ObjectID); ok {
		return id.Hex()
	}
	return fmt.Sprint(d["_id"])
}

func str(d bson.M, k string) string {
	s, _ := d[k].(string)
	return s
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

func dateOf(v any) time.Time {
	if dt, ok := v.(primitive.DateTime); ok {
		return dt.Time()
	}
	return time.Time{}
}

func arrLen(v any) int {
	if a, ok := v.(bson.A); ok {
		return len(a)
	}
	return 0
}
