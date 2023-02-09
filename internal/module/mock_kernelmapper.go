// Code generated by MockGen. DO NOT EDIT.
// Source: kernelmapper.go

// Package module is a generated GoMock package.
package module

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
)

// MockKernelMapper is a mock of KernelMapper interface.
type MockKernelMapper struct {
	ctrl     *gomock.Controller
	recorder *MockKernelMapperMockRecorder
}

// MockKernelMapperMockRecorder is the mock recorder for MockKernelMapper.
type MockKernelMapperMockRecorder struct {
	mock *MockKernelMapper
}

// NewMockKernelMapper creates a new mock instance.
func NewMockKernelMapper(ctrl *gomock.Controller) *MockKernelMapper {
	mock := &MockKernelMapper{ctrl: ctrl}
	mock.recorder = &MockKernelMapperMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockKernelMapper) EXPECT() *MockKernelMapperMockRecorder {
	return m.recorder
}

// GetMergedMappingForKernel mocks base method.
func (m *MockKernelMapper) GetMergedMappingForKernel(modSpec *v1beta1.ModuleSpec, kernelVersion string) (*v1beta1.KernelMapping, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMergedMappingForKernel", modSpec, kernelVersion)
	ret0, _ := ret[0].(*v1beta1.KernelMapping)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetMergedMappingForKernel indicates an expected call of GetMergedMappingForKernel.
func (mr *MockKernelMapperMockRecorder) GetMergedMappingForKernel(modSpec, kernelVersion interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMergedMappingForKernel", reflect.TypeOf((*MockKernelMapper)(nil).GetMergedMappingForKernel), modSpec, kernelVersion)
}

// MockkernelMapperHelperAPI is a mock of kernelMapperHelperAPI interface.
type MockkernelMapperHelperAPI struct {
	ctrl     *gomock.Controller
	recorder *MockkernelMapperHelperAPIMockRecorder
}

// MockkernelMapperHelperAPIMockRecorder is the mock recorder for MockkernelMapperHelperAPI.
type MockkernelMapperHelperAPIMockRecorder struct {
	mock *MockkernelMapperHelperAPI
}

// NewMockkernelMapperHelperAPI creates a new mock instance.
func NewMockkernelMapperHelperAPI(ctrl *gomock.Controller) *MockkernelMapperHelperAPI {
	mock := &MockkernelMapperHelperAPI{ctrl: ctrl}
	mock.recorder = &MockkernelMapperHelperAPIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockkernelMapperHelperAPI) EXPECT() *MockkernelMapperHelperAPIMockRecorder {
	return m.recorder
}

// findKernelMapping mocks base method.
func (m *MockkernelMapperHelperAPI) findKernelMapping(mappings []v1beta1.KernelMapping, kernelVersion string) (*v1beta1.KernelMapping, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "findKernelMapping", mappings, kernelVersion)
	ret0, _ := ret[0].(*v1beta1.KernelMapping)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// findKernelMapping indicates an expected call of findKernelMapping.
func (mr *MockkernelMapperHelperAPIMockRecorder) findKernelMapping(mappings, kernelVersion interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "findKernelMapping", reflect.TypeOf((*MockkernelMapperHelperAPI)(nil).findKernelMapping), mappings, kernelVersion)
}

// mergeMappingData mocks base method.
func (m *MockkernelMapperHelperAPI) mergeMappingData(mapping *v1beta1.KernelMapping, modSpec *v1beta1.ModuleSpec, kernelVersion string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "mergeMappingData", mapping, modSpec, kernelVersion)
	ret0, _ := ret[0].(error)
	return ret0
}

// mergeMappingData indicates an expected call of mergeMappingData.
func (mr *MockkernelMapperHelperAPIMockRecorder) mergeMappingData(mapping, modSpec, kernelVersion interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mergeMappingData", reflect.TypeOf((*MockkernelMapperHelperAPI)(nil).mergeMappingData), mapping, modSpec, kernelVersion)
}

// replaceTemplates mocks base method.
func (m *MockkernelMapperHelperAPI) replaceTemplates(mapping *v1beta1.KernelMapping, kernelVersion string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "replaceTemplates", mapping, kernelVersion)
	ret0, _ := ret[0].(error)
	return ret0
}

// replaceTemplates indicates an expected call of replaceTemplates.
func (mr *MockkernelMapperHelperAPIMockRecorder) replaceTemplates(mapping, kernelVersion interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "replaceTemplates", reflect.TypeOf((*MockkernelMapperHelperAPI)(nil).replaceTemplates), mapping, kernelVersion)
}
