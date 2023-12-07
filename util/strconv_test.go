package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_stringToAmount(t *testing.T) {
	type args struct {
		amount   string
		decimals uint32
	}
	tests := []struct {
		name       string
		args       args
		want       uint64
		wantErrStr string
	}{
		{
			name:       "empty",
			args:       args{amount: "", decimals: 2},
			want:       0,
			wantErrStr: "invalid empty amount string",
		},
		{
			name: "100.23, decimals 2 - ok",
			args: args{amount: "100.23", decimals: 2},
			want: 10023,
		},
		{
			name:       "100.2.3 error - too many commas",
			args:       args{amount: "100.2.3", decimals: 2},
			want:       0,
			wantErrStr: "more than one comma",
		},
		{
			name:       ".30 error - no whole number",
			args:       args{amount: ".3", decimals: 2},
			want:       0,
			wantErrStr: "missing integer part",
		},
		{
			name:       "30. error - no fraction",
			args:       args{amount: "30.", decimals: 2},
			want:       0,
			wantErrStr: "missing fraction part",
		},
		{
			name:       "1.000, decimals 2 - error invalid precision",
			args:       args{amount: "1.000", decimals: 2},
			want:       0,
			wantErrStr: "invalid precision",
		},
		{
			name:       "in.300, decimals 3 - error not number",
			args:       args{amount: "in.300", decimals: 3},
			want:       0,
			wantErrStr: "invalid amount string \"in.300\": error conversion to uint64 failed",
		},
		{
			name:       "12.3c0, decimals 3 - error not number",
			args:       args{amount: "12.3c0", decimals: 3},
			want:       0,
			wantErrStr: "invalid amount string \"12.3c0\": error conversion to uint64 failed",
		},
		{
			name: "2.30, decimals 2 - ok",
			args: args{amount: "2.30", decimals: 2},
			want: 230,
		},
		{
			name: "0000000.3, decimals 2 - ok",
			args: args{amount: "0000000.3", decimals: 2},
			want: 30,
		},
		{
			name: "0.300, decimals 3 - ok",
			args: args{amount: "0.300", decimals: 3},
			want: 300,
		},
		{
			name: "100.23, decimals 3 - ok",
			args: args{amount: "100.23", decimals: 3},
			want: 100230,
		},
		{
			name:       "18446744073709551615.2 out of range - error",
			args:       args{amount: "18446744073709551615.2", decimals: 1},
			want:       0,
			wantErrStr: "value out of range",
		},
		{
			name:       "18446744073709551616 out of range - error",
			args:       args{amount: "18446744073709551616", decimals: 0},
			want:       0,
			wantErrStr: "value out of range",
		},
		{
			name:       "18446744073709551615 out of range - error",
			args:       args{amount: "18446744073709551615", decimals: 1},
			want:       0,
			wantErrStr: "value out of range",
		},
		{
			name: "18446744073709551615 max - ok",
			args: args{amount: "18446744073709551615", decimals: 0},
			want: 18446744073709551615,
		},
		{
			name: "184467440737.09551615 max with decimals - ok",
			args: args{amount: "184467440737.09551615", decimals: 8},
			want: 18446744073709551615,
		},
		{
			name: "10'000.234'5, decimals 4 - ok",
			args: args{amount: "10'000.234'5", decimals: 4},
			want: 100002345,
		},
		{
			name: "1'00'00.2'34'5, misplaced separators ignored - ok",
			args: args{amount: "1'00'00.2'34'5", decimals: 4},
			want: 100002345,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StringToAmount(tt.args.amount, tt.args.decimals)
			if len(tt.wantErrStr) > 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Equal(t, uint64(0), got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_amountToString(t *testing.T) {
	type args struct {
		amount    uint64
		decPlaces uint32
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Conversion ok - decimals 2",
			args: args{amount: 12345, decPlaces: 2},
			want: "123.45",
		},
		{
			name: "Conversion ok - decimals 1",
			args: args{amount: 12345, decPlaces: 1},
			want: "1'234.5",
		},
		{
			name: "Conversion ok - decimals 0",
			args: args{amount: 12345, decPlaces: 0},
			want: "12'345",
		},
		{
			name: "Conversion ok - decimals 7",
			args: args{amount: 12345, decPlaces: 5},
			want: "0.123'45",
		},
		{
			name: "Conversion ok - decimals 9",
			args: args{amount: 12345, decPlaces: 9},
			want: "0.000'012'345",
		},
		{
			name: "Conversion ok - 99999 ",
			args: args{amount: 99999, decPlaces: 7},
			want: "0.009'999'9",
		},
		{
			name: "Conversion ok - 9000 ",
			args: args{amount: 9000, decPlaces: 5},
			want: "0.090'00",
		},
		{
			name: "Conversion ok - 3 ",
			args: args{amount: 3, decPlaces: 2},
			want: "0.03",
		},
		{
			name: "Conversion ok - 3 ",
			args: args{amount: 3, decPlaces: 2},
			want: "0.03",
		},
		{
			name: "Conversion of max - 18446744073709551615 ",
			args: args{amount: 18446744073709551615, decPlaces: 8},
			want: "184'467'440'737.095'516'15",
		},
		{
			name: "decimals out of bounds - 18446744073709551615 ",
			args: args{amount: 18446744073709551615, decPlaces: 32},
			want: "0.000'000'000'000'184'467'440'737'095'516'15",
		},
		{
			name: "Conversion ok - 2.30",
			args: args{amount: 230, decPlaces: 2},
			want: "2.30",
		},
		{
			name: "Conversion ok - 0.300",
			args: args{amount: 3, decPlaces: 3},
			want: "0.003",
		},
		{
			name: "Conversion ok - 100230, decimals 3 - ok",
			args: args{amount: 100230, decPlaces: 3},
			want: "100.230",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AmountToString(tt.args.amount, tt.args.decPlaces)
			if got != tt.want {
				t.Errorf("amountToString() got = %v, want %v", got, tt.want)
			}
		})
	}
}
