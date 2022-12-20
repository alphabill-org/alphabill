package distributed

import (
	"reflect"
	"testing"
)

func TestIrReqBuffer_Add(t *testing.T) {
	type fields struct {
		irChgReqBuffer map[protocol.SystemIdentifier]*IRChange
	}
	type args struct {
		req      *atomic_broadcast.IRChangeReqMsg
		luc      *certificates.UnicityCertificate
		nofNodes int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IrReqBuffer{
				irChgReqBuffer: tt.fields.irChgReqBuffer,
			}
			if err := x.Add(tt.args.req, tt.args.luc, tt.args.nofNodes); (err != nil) != tt.wantErr {
				t.Errorf("Add() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIrReqBuffer_GeneratePayload(t *testing.T) {
	type fields struct {
		irChgReqBuffer map[protocol.SystemIdentifier]*IRChange
	}
	tests := []struct {
		name   string
		fields fields
		want   *atomic_broadcast.Payload
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IrReqBuffer{
				irChgReqBuffer: tt.fields.irChgReqBuffer,
			}
			if got := x.GeneratePayload(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GeneratePayload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewIrReqBuffer(t *testing.T) {
	tests := []struct {
		name string
		want *IrReqBuffer
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewIrReqBuffer(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewIrReqBuffer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_compareIR(t *testing.T) {
	type args struct {
		a *certificates.InputRecord
		b *certificates.InputRecord
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareIR(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("compareIR() = %v, want %v", got, tt.want)
			}
		})
	}
}
