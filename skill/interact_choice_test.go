package skill

import "testing"

func TestTalkApproachChoiceIndexOnlyHandlesMuseumAdmission(t *testing.T) {
	tests := []struct {
		name  string
		mapID uint8
		text  string
		want  bool
	}{
		{
			name:  "live museum prompt",
			mapID: museum1FMap,
			text:  "MONEY ¥1623 YES NO Would you like to come in?",
			want:  true,
		},
		{
			name:  "wrapped museum prompt",
			mapID: museum1FMap,
			text:  "MONEY ¥1623 YES NO\nWould you like to\ncome in?",
			want:  true,
		},
		{
			name:  "same words on another map",
			mapID: 0x01,
			text:  "Would you like to come in?",
			want:  false,
		},
		{
			name:  "unrelated museum yes no",
			mapID: museum1FMap,
			text:  "YES NO Do you know what AMBER is?",
			want:  false,
		},
		{
			name:  "generic nickname prompt",
			mapID: museum1FMap,
			text:  "YES NO Give a nickname?",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, ok := talkApproachChoiceIndex(tt.mapID, tt.text)
			if ok != tt.want {
				t.Fatalf("talkApproachChoiceIndex(%#04x, %q) ok = %v, want %v", tt.mapID, tt.text, ok, tt.want)
			}
			if ok && index != 0 {
				t.Fatalf("museum admission index = %d, want 0 (YES)", index)
			}
		})
	}
}
