package api_test

import (
	"strings"
	"testing"
)

// Adding someone who is already on a hotel's team changes nothing and says so: an owner cannot be demoted by mistake.
func TestAddingATeamMemberAgainChangesNothing(t *testing.T) {
	admin := adminClient(t)
	path := "/admin/hotels/" + meskel + "/staff"
	mail := uniqueEmail("twice")
	if r := admin.do("POST", path, map[string]string{"contact": mail, "role": "owner"}); r.status != 200 {
		t.Fatalf("first add: %d %s", r.status, r.raw)
	}
	t.Cleanup(func() {
		r := admin.do("GET", "/admin/hotels/"+meskel, nil)
		for _, p := range r.body["staff"].([]any) {
			if m := p.(map[string]any); m["contact"] == mail {
				admin.do("DELETE", path+"/"+m["user_id"].(string), nil)
			}
		}
	})
	for _, role := range []string{"receptionist", "owner"} {
		r := admin.do("POST", path, map[string]string{"contact": mail, "role": role})
		if r.status != 409 || !strings.Contains(r.str("error", "message"), "already on this hotel's team as an owner") {
			t.Fatalf("adding again as %s: %d %s", role, r.status, r.raw)
		}
	}
	var found string
	for _, p := range admin.do("GET", "/admin/hotels/"+meskel, nil).body["staff"].([]any) {
		if m := p.(map[string]any); m["contact"] == mail {
			found = m["role"].(string)
		}
	}
	if found != "owner" {
		t.Fatalf("the role changed to %q", found)
	}
	if r := admin.do("POST", path, map[string]string{"contact": "not-an-email", "role": "receptionist"}); r.status != 400 || !strings.Contains(r.str("error", "message"), "email address or") {
		t.Fatalf("a mistyped contact: %d %s", r.status, r.raw)
	}
}
