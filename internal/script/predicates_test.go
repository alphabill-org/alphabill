package _s

//
//func TestExtractPubKeyFromPredicateArgument(t *testing.T) {
//	type args struct {
//		predicate []byte
//	}
//	txBytes := []byte{1, 2, 3, 4, 5}
//	sig, pubKey := testsig.SignBytes(t, txBytes)
//	tests := []struct {
//		name       string
//		args       args
//		want       []byte
//		wantErrStr string
//	}{
//		{
//			name:       "argument nil",
//			args:       args{predicate: nil},
//			wantErrStr: "predicate argument is nil",
//		},
//		{
//			name:       "argument 0 bytes",
//			args:       args{predicate: []byte{}},
//			wantErrStr: "invalid predicate argument",
//		},
//		{
//			name:       "argument 1 byte, but not start byte",
//			args:       args{predicate: []byte{0xaa}},
//			wantErrStr: "invalid predicate argument",
//		},
//		{
//			name:       "argument only start byte",
//			args:       args{predicate: PredicateArgumentEmpty()},
//			wantErrStr: "invalid predicate argument",
//		},
//		{
//			name:       "argument always true",
//			args:       args{predicate: PredicateAlwaysTrue()},
//			wantErrStr: "no public key found in predicate argument",
//		},
//		{
//			name:       "argument always false",
//			args:       args{predicate: PredicateAlwaysFalse()},
//			wantErrStr: "no public key found in predicate argument",
//		},
//		{
//			name:       "argument does not start with StartByte",
//			args:       args{predicate: []byte{4, 5, 6, 7, 8}},
//			wantErrStr: "invalid predicate argument",
//		},
//		{
//			name:       "argument incorrect signature length",
//			args:       args{predicate: PredicateArgumentPayToPublicKeyHashDefault(make([]byte, 32), make([]byte, 44, 0xff))},
//			wantErrStr: "unknown opcode",
//		},
//		{
//			name: "argument pay to public key hash",
//			args: args{PredicateArgumentPayToPublicKeyHashDefault(sig, pubKey)},
//			want: pubKey,
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			got, err := ExtractPubKeyFromPredicateArgument(tt.args.predicate)
//			if tt.wantErrStr != "" {
//				require.ErrorContains(t, err, tt.wantErrStr)
//				require.Nil(t, got)
//			} else {
//				require.NoError(t, err)
//				require.True(t, bytes.Equal(got, tt.want))
//			}
//		})
//	}
//}
