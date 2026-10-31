package persistence

import "testing"

func TestToMigrateURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{
			in:   "postgres://user:pass@localhost:5432/db?sslmode=disable",
			want: "pgx5://user:pass@localhost:5432/db?sslmode=disable",
		},
		{
			in:   "postgresql://user:pass@localhost:5432/db?sslmode=disable",
			want: "pgx5://user:pass@localhost:5432/db?sslmode=disable",
		},
		{
			in:   "pgx5://user:pass@localhost:5432/db?sslmode=disable",
			want: "pgx5://user:pass@localhost:5432/db?sslmode=disable",
		},
	}

	for _, tc := range cases {
		got := toMigrateURL(tc.in)
		if got != tc.want {
			t.Fatalf("toMigrateURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
