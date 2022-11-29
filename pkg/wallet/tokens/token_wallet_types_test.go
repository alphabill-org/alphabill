package tokens

import "testing"

func TestTokenKind_String(t *testing.T) {
	tests := []struct {
		name string
		k    TokenKind
		want string
	}{
		{
			name: "any",
			k:    Any,
			want: "any",
		},
		{
			name: "type",
			k:    TokenType,
			want: "type",
		},
		{
			name: "token",
			k:    Token,
			want: "token",
		},
		{
			name: "fungible token",
			k:    FungibleToken,
			want: "token,fungible",
		},
		{
			name: "nft token",
			k:    NonFungibleToken,
			want: "token,non-fungible",
		},
		{
			name: "fungible type",
			k:    FungibleTokenType,
			want: "type,fungible",
		},
		{
			name: "nft type",
			k:    NonFungibleTokenType,
			want: "type,non-fungible",
		},
		{
			name: "absurd, but not stringer problem",
			k:    TokenType | Token | Fungible | NonFungible,
			want: "type,token,fungible,non-fungible",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.k.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
