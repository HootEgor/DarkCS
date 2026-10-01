package bot

import "testing"

func TestClassifyBusinessMessage(t *testing.T) {
	const owner, client, ourBot, otherBot = 10, 20, 30, 40
	tests := []struct {
		name                string
		from, senderBot     int64
		wantDir, wantSender string
		wantSkip            bool
	}{
		{"client message", client, 0, "incoming", "user", false},
		{"owner typed in app", owner, 0, "outgoing", "manager", false},
		{"echo of our CRM reply", owner, ourBot, "", "", true},
		{"another bot of the owner", owner, otherBot, "outgoing", "manager", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, sender, skip := classifyBusinessMessage(tt.from, owner, tt.senderBot, ourBot)
			if dir != tt.wantDir || sender != tt.wantSender || skip != tt.wantSkip {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", dir, sender, skip, tt.wantDir, tt.wantSender, tt.wantSkip)
			}
		})
	}
}
