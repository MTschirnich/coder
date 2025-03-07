package dbfake_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
)

// test that transactions don't deadlock, and that we don't see intermediate state.
func TestInTx(t *testing.T) {
	t.Parallel()

	uut := dbfake.New()

	inTx := make(chan any)
	queriesDone := make(chan any)
	queriesStarted := make(chan any)
	go func() {
		err := uut.InTx(func(tx database.Store) error {
			close(inTx)
			_, err := tx.InsertOrganization(context.Background(), database.InsertOrganizationParams{
				Name: "1",
			})
			assert.NoError(t, err)
			<-queriesStarted
			time.Sleep(5 * time.Millisecond)
			_, err = tx.InsertOrganization(context.Background(), database.InsertOrganizationParams{
				Name: "2",
			})
			assert.NoError(t, err)
			return nil
		}, nil)
		assert.NoError(t, err)
	}()
	var nums []int
	go func() {
		<-inTx
		for i := 0; i < 20; i++ {
			orgs, err := uut.GetOrganizations(context.Background())
			if err != nil {
				assert.ErrorIs(t, err, sql.ErrNoRows)
			}
			nums = append(nums, len(orgs))
			time.Sleep(time.Millisecond)
		}
		close(queriesDone)
	}()
	close(queriesStarted)
	<-queriesDone
	// ensure we never saw 1 org, only 0 or 2.
	for i := 0; i < 20; i++ {
		assert.NotEqual(t, 1, nums[i])
	}
}

// TestExactMethods will ensure the fake database does not hold onto excessive
// functions. The fake database is a manual implementation, so it is possible
// we forget to delete functions that we remove. This unit test just ensures
// we remove the extra methods.
func TestExactMethods(t *testing.T) {
	t.Parallel()

	// extraFakeMethods contains the extra allowed methods that are not a part
	// of the database.Store interface.
	extraFakeMethods := map[string]string{
		// Example
		// "SortFakeLists": "Helper function used",
		"IsFakeDB": "Helper function used for unit testing",
	}

	fake := reflect.TypeOf(dbfake.New())
	fakeMethods := methods(fake)

	store := reflect.TypeOf((*database.Store)(nil)).Elem()
	storeMethods := methods(store)

	// Store should be a subset
	for k := range storeMethods {
		_, ok := fakeMethods[k]
		if !ok {
			panic(fmt.Sprintf("This should never happen. FakeDB missing method %s, so doesn't fit the interface", k))
		}
		delete(storeMethods, k)
		delete(fakeMethods, k)
	}

	for k := range fakeMethods {
		_, ok := extraFakeMethods[k]
		if ok {
			continue
		}
		// If you are seeing this error, you have an extra function not required
		// for the database.Store. If you still want to keep it, add it to
		// 'extraFakeMethods' to allow it.
		t.Errorf("Fake method '%s()' is excessive and not needed to fit interface, delete it", k)
	}
}

// TestUserOrder ensures that the fake database returns users in the order they
// were created.
func TestUserOrder(t *testing.T) {
	t.Parallel()

	db := dbfake.New()
	now := database.Now()
	count := 10
	for i := 0; i < count; i++ {
		dbgen.User(t, db, database.User{
			Username:  fmt.Sprintf("user%d", i),
			CreatedAt: now,
		})
	}

	users, err := db.GetUsers(context.Background(), database.GetUsersParams{})
	require.NoError(t, err)
	require.Lenf(t, users, count, "expected %d users", count)
	for i, user := range users {
		require.Equal(t, fmt.Sprintf("user%d", i), user.Username)
	}
}

func TestProxyByHostname(t *testing.T) {
	t.Parallel()

	db := dbfake.New()

	// Insert a bunch of different proxies.
	proxies := []struct {
		name             string
		accessURL        string
		wildcardHostname string
	}{
		{
			name:             "one",
			accessURL:        "https://one.coder.com",
			wildcardHostname: "*.wildcard.one.coder.com",
		},
		{
			name:             "two",
			accessURL:        "https://two.coder.com",
			wildcardHostname: "*--suffix.two.coder.com",
		},
	}
	for _, p := range proxies {
		dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{
			Name:             p.name,
			Url:              p.accessURL,
			WildcardHostname: p.wildcardHostname,
		})
	}

	cases := []struct {
		name           string
		testHostname   string
		matchProxyName string
	}{
		{
			name:           "NoMatch",
			testHostname:   "test.com",
			matchProxyName: "",
		},
		{
			name:           "MatchAccessURL",
			testHostname:   "one.coder.com",
			matchProxyName: "one",
		},
		{
			name:           "MatchWildcard",
			testHostname:   "something.wildcard.one.coder.com",
			matchProxyName: "one",
		},
		{
			name:           "MatchSuffix",
			testHostname:   "something--suffix.two.coder.com",
			matchProxyName: "two",
		},
		{
			name:           "ValidateHostname/1",
			testHostname:   ".*ne.coder.com",
			matchProxyName: "",
		},
		{
			name:           "ValidateHostname/2",
			testHostname:   "https://one.coder.com",
			matchProxyName: "",
		},
		{
			name:           "ValidateHostname/3",
			testHostname:   "one.coder.com:8080/hello",
			matchProxyName: "",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			proxy, err := db.GetWorkspaceProxyByHostname(context.Background(), c.testHostname)
			if c.matchProxyName == "" {
				require.ErrorIs(t, err, sql.ErrNoRows)
				require.Empty(t, proxy)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, proxy)
				require.Equal(t, c.matchProxyName, proxy.Name)
			}
		})
	}
}

func methods(rt reflect.Type) map[string]bool {
	methods := make(map[string]bool)
	for i := 0; i < rt.NumMethod(); i++ {
		methods[rt.Method(i).Name] = true
	}
	return methods
}
