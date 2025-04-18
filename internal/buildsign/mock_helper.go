// Code generated by MockGen. DO NOT EDIT.
// Source: helper.go
//
// Generated by this command:
//
//	mockgen -source=helper.go -package=buildsign -destination=mock_helper.go
//
// Package buildsign is a generated GoMock package.
package buildsign

import (
	reflect "reflect"

	v1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	gomock "go.uber.org/mock/gomock"
)

// MockHelper is a mock of Helper interface.
type MockHelper struct {
	ctrl     *gomock.Controller
	recorder *MockHelperMockRecorder
}

// MockHelperMockRecorder is the mock recorder for MockHelper.
type MockHelperMockRecorder struct {
	mock *MockHelper
}

// NewMockHelper creates a new mock instance.
func NewMockHelper(ctrl *gomock.Controller) *MockHelper {
	mock := &MockHelper{ctrl: ctrl}
	mock.recorder = &MockHelperMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHelper) EXPECT() *MockHelperMockRecorder {
	return m.recorder
}

// ApplyBuildArgOverrides mocks base method.
func (m *MockHelper) ApplyBuildArgOverrides(args []v1beta1.BuildArg, overrides ...v1beta1.BuildArg) []v1beta1.BuildArg {
	m.ctrl.T.Helper()
	varargs := []any{args}
	for _, a := range overrides {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "ApplyBuildArgOverrides", varargs...)
	ret0, _ := ret[0].([]v1beta1.BuildArg)
	return ret0
}

// ApplyBuildArgOverrides indicates an expected call of ApplyBuildArgOverrides.
func (mr *MockHelperMockRecorder) ApplyBuildArgOverrides(args any, overrides ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{args}, overrides...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ApplyBuildArgOverrides", reflect.TypeOf((*MockHelper)(nil).ApplyBuildArgOverrides), varargs...)
}

// GetRelevantBuild mocks base method.
func (m *MockHelper) GetRelevantBuild(moduleBuild, mappingBuild *v1beta1.Build) *v1beta1.Build {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRelevantBuild", moduleBuild, mappingBuild)
	ret0, _ := ret[0].(*v1beta1.Build)
	return ret0
}

// GetRelevantBuild indicates an expected call of GetRelevantBuild.
func (mr *MockHelperMockRecorder) GetRelevantBuild(moduleBuild, mappingBuild any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRelevantBuild", reflect.TypeOf((*MockHelper)(nil).GetRelevantBuild), moduleBuild, mappingBuild)
}

// GetRelevantSign mocks base method.
func (m *MockHelper) GetRelevantSign(moduleSign, mappingSign *v1beta1.Sign, kernel string) (*v1beta1.Sign, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRelevantSign", moduleSign, mappingSign, kernel)
	ret0, _ := ret[0].(*v1beta1.Sign)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRelevantSign indicates an expected call of GetRelevantSign.
func (mr *MockHelperMockRecorder) GetRelevantSign(moduleSign, mappingSign, kernel any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRelevantSign", reflect.TypeOf((*MockHelper)(nil).GetRelevantSign), moduleSign, mappingSign, kernel)
}
