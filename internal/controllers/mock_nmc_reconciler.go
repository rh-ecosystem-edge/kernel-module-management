// Code generated by MockGen. DO NOT EDIT.
// Source: nmc_reconciler.go
//
// Generated by this command:
//
//	mockgen -source=nmc_reconciler.go -package=controllers -destination=mock_nmc_reconciler.go workerHelper
//
// Package controllers is a generated GoMock package.
package controllers

import (
	context "context"
	reflect "reflect"

	v1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	gomock "go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	types "k8s.io/apimachinery/pkg/types"
	sets "k8s.io/apimachinery/pkg/util/sets"
)

// MocknmcReconcilerHelper is a mock of nmcReconcilerHelper interface.
type MocknmcReconcilerHelper struct {
	ctrl     *gomock.Controller
	recorder *MocknmcReconcilerHelperMockRecorder
}

// MocknmcReconcilerHelperMockRecorder is the mock recorder for MocknmcReconcilerHelper.
type MocknmcReconcilerHelperMockRecorder struct {
	mock *MocknmcReconcilerHelper
}

// NewMocknmcReconcilerHelper creates a new mock instance.
func NewMocknmcReconcilerHelper(ctrl *gomock.Controller) *MocknmcReconcilerHelper {
	mock := &MocknmcReconcilerHelper{ctrl: ctrl}
	mock.recorder = &MocknmcReconcilerHelperMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MocknmcReconcilerHelper) EXPECT() *MocknmcReconcilerHelperMockRecorder {
	return m.recorder
}

// GarbageCollectInUseLabels mocks base method.
func (m *MocknmcReconcilerHelper) GarbageCollectInUseLabels(ctx context.Context, nmc *v1beta1.NodeModulesConfig) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GarbageCollectInUseLabels", ctx, nmc)
	ret0, _ := ret[0].(error)
	return ret0
}

// GarbageCollectInUseLabels indicates an expected call of GarbageCollectInUseLabels.
func (mr *MocknmcReconcilerHelperMockRecorder) GarbageCollectInUseLabels(ctx, nmc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GarbageCollectInUseLabels", reflect.TypeOf((*MocknmcReconcilerHelper)(nil).GarbageCollectInUseLabels), ctx, nmc)
}

// ProcessModuleSpec mocks base method.
func (m *MocknmcReconcilerHelper) ProcessModuleSpec(ctx context.Context, nmc *v1beta1.NodeModulesConfig, spec *v1beta1.NodeModuleSpec, status *v1beta1.NodeModuleStatus, node *v1.Node) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ProcessModuleSpec", ctx, nmc, spec, status, node)
	ret0, _ := ret[0].(error)
	return ret0
}

// ProcessModuleSpec indicates an expected call of ProcessModuleSpec.
func (mr *MocknmcReconcilerHelperMockRecorder) ProcessModuleSpec(ctx, nmc, spec, status, node any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ProcessModuleSpec", reflect.TypeOf((*MocknmcReconcilerHelper)(nil).ProcessModuleSpec), ctx, nmc, spec, status, node)
}

// ProcessUnconfiguredModuleStatus mocks base method.
func (m *MocknmcReconcilerHelper) ProcessUnconfiguredModuleStatus(ctx context.Context, nmc *v1beta1.NodeModulesConfig, status *v1beta1.NodeModuleStatus, node *v1.Node) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ProcessUnconfiguredModuleStatus", ctx, nmc, status, node)
	ret0, _ := ret[0].(error)
	return ret0
}

// ProcessUnconfiguredModuleStatus indicates an expected call of ProcessUnconfiguredModuleStatus.
func (mr *MocknmcReconcilerHelperMockRecorder) ProcessUnconfiguredModuleStatus(ctx, nmc, status, node any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ProcessUnconfiguredModuleStatus", reflect.TypeOf((*MocknmcReconcilerHelper)(nil).ProcessUnconfiguredModuleStatus), ctx, nmc, status, node)
}

// RecordEvents mocks base method.
func (m *MocknmcReconcilerHelper) RecordEvents(node *v1.Node, loadedModules, unloadedModules []types.NamespacedName) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RecordEvents", node, loadedModules, unloadedModules)
}

// RecordEvents indicates an expected call of RecordEvents.
func (mr *MocknmcReconcilerHelperMockRecorder) RecordEvents(node, loadedModules, unloadedModules any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RecordEvents", reflect.TypeOf((*MocknmcReconcilerHelper)(nil).RecordEvents), node, loadedModules, unloadedModules)
}

// RemovePodFinalizers mocks base method.
func (m *MocknmcReconcilerHelper) RemovePodFinalizers(ctx context.Context, nodeName string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemovePodFinalizers", ctx, nodeName)
	ret0, _ := ret[0].(error)
	return ret0
}

// RemovePodFinalizers indicates an expected call of RemovePodFinalizers.
func (mr *MocknmcReconcilerHelperMockRecorder) RemovePodFinalizers(ctx, nodeName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemovePodFinalizers", reflect.TypeOf((*MocknmcReconcilerHelper)(nil).RemovePodFinalizers), ctx, nodeName)
}

// SyncStatus mocks base method.
func (m *MocknmcReconcilerHelper) SyncStatus(ctx context.Context, nmc *v1beta1.NodeModulesConfig) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SyncStatus", ctx, nmc)
	ret0, _ := ret[0].(error)
	return ret0
}

// SyncStatus indicates an expected call of SyncStatus.
func (mr *MocknmcReconcilerHelperMockRecorder) SyncStatus(ctx, nmc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SyncStatus", reflect.TypeOf((*MocknmcReconcilerHelper)(nil).SyncStatus), ctx, nmc)
}

// UpdateNodeLabels mocks base method.
func (m *MocknmcReconcilerHelper) UpdateNodeLabels(ctx context.Context, nmc *v1beta1.NodeModulesConfig, node *v1.Node) ([]types.NamespacedName, []types.NamespacedName, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateNodeLabels", ctx, nmc, node)
	ret0, _ := ret[0].([]types.NamespacedName)
	ret1, _ := ret[1].([]types.NamespacedName)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// UpdateNodeLabels indicates an expected call of UpdateNodeLabels.
func (mr *MocknmcReconcilerHelperMockRecorder) UpdateNodeLabels(ctx, nmc, node any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateNodeLabels", reflect.TypeOf((*MocknmcReconcilerHelper)(nil).UpdateNodeLabels), ctx, nmc, node)
}

// MocklabelPreparationHelper is a mock of labelPreparationHelper interface.
type MocklabelPreparationHelper struct {
	ctrl     *gomock.Controller
	recorder *MocklabelPreparationHelperMockRecorder
}

// MocklabelPreparationHelperMockRecorder is the mock recorder for MocklabelPreparationHelper.
type MocklabelPreparationHelperMockRecorder struct {
	mock *MocklabelPreparationHelper
}

// NewMocklabelPreparationHelper creates a new mock instance.
func NewMocklabelPreparationHelper(ctrl *gomock.Controller) *MocklabelPreparationHelper {
	mock := &MocklabelPreparationHelper{ctrl: ctrl}
	mock.recorder = &MocklabelPreparationHelperMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MocklabelPreparationHelper) EXPECT() *MocklabelPreparationHelperMockRecorder {
	return m.recorder
}

// addEqualLabels mocks base method.
func (m *MocklabelPreparationHelper) addEqualLabels(nodeModuleReadyLabels sets.Set[types.NamespacedName], specLabels, statusLabels map[types.NamespacedName]v1beta1.ModuleConfig) []types.NamespacedName {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "addEqualLabels", nodeModuleReadyLabels, specLabels, statusLabels)
	ret0, _ := ret[0].([]types.NamespacedName)
	return ret0
}

// addEqualLabels indicates an expected call of addEqualLabels.
func (mr *MocklabelPreparationHelperMockRecorder) addEqualLabels(nodeModuleReadyLabels, specLabels, statusLabels any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "addEqualLabels", reflect.TypeOf((*MocklabelPreparationHelper)(nil).addEqualLabels), nodeModuleReadyLabels, specLabels, statusLabels)
}

// getDeprecatedKernelModuleReadyLabels mocks base method.
func (m *MocklabelPreparationHelper) getDeprecatedKernelModuleReadyLabels(node v1.Node) sets.Set[string] {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getDeprecatedKernelModuleReadyLabels", node)
	ret0, _ := ret[0].(sets.Set[string])
	return ret0
}

// getDeprecatedKernelModuleReadyLabels indicates an expected call of getDeprecatedKernelModuleReadyLabels.
func (mr *MocklabelPreparationHelperMockRecorder) getDeprecatedKernelModuleReadyLabels(node any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getDeprecatedKernelModuleReadyLabels", reflect.TypeOf((*MocklabelPreparationHelper)(nil).getDeprecatedKernelModuleReadyLabels), node)
}

// getNodeKernelModuleReadyLabels mocks base method.
func (m *MocklabelPreparationHelper) getNodeKernelModuleReadyLabels(node v1.Node) sets.Set[types.NamespacedName] {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getNodeKernelModuleReadyLabels", node)
	ret0, _ := ret[0].(sets.Set[types.NamespacedName])
	return ret0
}

// getNodeKernelModuleReadyLabels indicates an expected call of getNodeKernelModuleReadyLabels.
func (mr *MocklabelPreparationHelperMockRecorder) getNodeKernelModuleReadyLabels(node any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getNodeKernelModuleReadyLabels", reflect.TypeOf((*MocklabelPreparationHelper)(nil).getNodeKernelModuleReadyLabels), node)
}

// getSpecLabelsAndTheirConfigs mocks base method.
func (m *MocklabelPreparationHelper) getSpecLabelsAndTheirConfigs(nmc *v1beta1.NodeModulesConfig) map[types.NamespacedName]v1beta1.ModuleConfig {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getSpecLabelsAndTheirConfigs", nmc)
	ret0, _ := ret[0].(map[types.NamespacedName]v1beta1.ModuleConfig)
	return ret0
}

// getSpecLabelsAndTheirConfigs indicates an expected call of getSpecLabelsAndTheirConfigs.
func (mr *MocklabelPreparationHelperMockRecorder) getSpecLabelsAndTheirConfigs(nmc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getSpecLabelsAndTheirConfigs", reflect.TypeOf((*MocklabelPreparationHelper)(nil).getSpecLabelsAndTheirConfigs), nmc)
}

// getStatusLabelsAndTheirConfigs mocks base method.
func (m *MocklabelPreparationHelper) getStatusLabelsAndTheirConfigs(nmc *v1beta1.NodeModulesConfig) map[types.NamespacedName]v1beta1.ModuleConfig {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getStatusLabelsAndTheirConfigs", nmc)
	ret0, _ := ret[0].(map[types.NamespacedName]v1beta1.ModuleConfig)
	return ret0
}

// getStatusLabelsAndTheirConfigs indicates an expected call of getStatusLabelsAndTheirConfigs.
func (mr *MocklabelPreparationHelperMockRecorder) getStatusLabelsAndTheirConfigs(nmc any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getStatusLabelsAndTheirConfigs", reflect.TypeOf((*MocklabelPreparationHelper)(nil).getStatusLabelsAndTheirConfigs), nmc)
}

// removeOrphanedLabels mocks base method.
func (m *MocklabelPreparationHelper) removeOrphanedLabels(nodeModuleReadyLabels sets.Set[types.NamespacedName], specLabels, statusLabels map[types.NamespacedName]v1beta1.ModuleConfig) []types.NamespacedName {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "removeOrphanedLabels", nodeModuleReadyLabels, specLabels, statusLabels)
	ret0, _ := ret[0].([]types.NamespacedName)
	return ret0
}

// removeOrphanedLabels indicates an expected call of removeOrphanedLabels.
func (mr *MocklabelPreparationHelperMockRecorder) removeOrphanedLabels(nodeModuleReadyLabels, specLabels, statusLabels any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "removeOrphanedLabels", reflect.TypeOf((*MocklabelPreparationHelper)(nil).removeOrphanedLabels), nodeModuleReadyLabels, specLabels, statusLabels)
}
