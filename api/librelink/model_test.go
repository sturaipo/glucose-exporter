package librelink

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_parseDate(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{
			name: "parse valid date",
			args: args{
				s: "9/7/2025 6:01:03 PM",
			},
			want:    time.Date(2025, 9, 7, 18, 1, 3, 0, time.UTC),
			wantErr: false,
		},
		{
			name: "parse malformed date",
			args: args{
				s: "19/7/2025 6:01:03 PM",
			},
			want:    time.Time{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDate(tt.args.s)

			if tt.wantErr {
				assert.Error(t, err, "expected an error for input: %s", tt.args.s)
				return
			}

			assert.NoError(t, err, "unexpected error for input: %s", tt.args.s)
			assert.Equal(t, tt.want, got, "parsed date does not match expected")
		})
	}
}

func Test_ParseJson(t *testing.T) {
	type args struct {
		s string
		t AuthTicket
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{
			name: "parse valid token",
			args: args{
				s: `{
      "token": "eyveioubn",
      "expires": 1773417313,
      "duration": 15552000000
			}`,
				t: AuthTicket{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			err := json.Unmarshal([]byte(tt.args.s), &tt.args.t)

			if tt.wantErr {
				require.Error(t, err, "expected an error for input: %s", tt.args.s)
				return
			}

			require.NoError(t, err, "unexpected error for input: %s", tt.args.s)
		})
	}
}
