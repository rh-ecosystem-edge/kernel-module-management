// Code generated by MockGen. DO NOT EDIT.
// Source: signer.go

// Package signjob is a generated GoMock package.
package signjob

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	v1 "k8s.io/api/batch/v1"
	v10 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockSigner is a mock of Signer interface.
type MockSigner struct {
	ctrl     *gomock.Controller
	recorder *MockSignerMockRecorder
}

// MockSignerMockRecorder is the mock recorder for MockSigner.
type MockSignerMockRecorder struct {
	mock *MockSigner
}

// NewMockSigner creates a new mock instance.
func NewMockSigner(ctrl *gomock.Controller) *MockSigner {
	mock := &MockSigner{ctrl: ctrl}
	mock.recorder = &MockSignerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSigner) EXPECT() *MockSignerMockRecorder {
	return m.recorder
}

// MakeJobTemplate mocks base method.
func (m *MockSigner) MakeJobTemplate(ctx context.Context, mod v1beta1.Module, km v1beta1.KernelMapping, targetKernel string, labels map[string]string, imageToSign string, pushImage bool, owner v10.Object) (*v1.Job, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MakeJobTemplate", ctx, mod, km, targetKernel, labels, imageToSign, pushImage, owner)
	ret0, _ := ret[0].(*v1.Job)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// MakeJobTemplate indicates an expected call of MakeJobTemplate.
func (mr *MockSignerMockRecorder) MakeJobTemplate(ctx, mod, km, targetKernel, labels, imageToSign, pushImage, owner interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MakeJobTemplate", reflect.TypeOf((*MockSigner)(nil).MakeJobTemplate), ctx, mod, km, targetKernel, labels, imageToSign, pushImage, owner)
}
