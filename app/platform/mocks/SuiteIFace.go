// Code generated by mockery v2.14.0. DO NOT EDIT.

// Regenerate this file using `make platform-mocks`.

package mocks

import (
	model "github.com/mattermost/mattermost-server/v6/model"
	mock "github.com/stretchr/testify/mock"
)

// SuiteIFace is an autogenerated mock type for the SuiteIFace type
type SuiteIFace struct {
	mock.Mock
}

// GetSession provides a mock function with given fields: token
func (_m *SuiteIFace) GetSession(token string) (*model.Session, *model.AppError) {
	ret := _m.Called(token)

	var r0 *model.Session
	if rf, ok := ret.Get(0).(func(string) *model.Session); ok {
		r0 = rf(token)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Session)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string) *model.AppError); ok {
		r1 = rf(token)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// IsUserAway provides a mock function with given fields: lastActivityAt
func (_m *SuiteIFace) IsUserAway(lastActivityAt int64) bool {
	ret := _m.Called(lastActivityAt)

	var r0 bool
	if rf, ok := ret.Get(0).(func(int64) bool); ok {
		r0 = rf(lastActivityAt)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// RolesGrantPermission provides a mock function with given fields: roleNames, permissionId
func (_m *SuiteIFace) RolesGrantPermission(roleNames []string, permissionId string) bool {
	ret := _m.Called(roleNames, permissionId)

	var r0 bool
	if rf, ok := ret.Get(0).(func([]string, string) bool); ok {
		r0 = rf(roleNames, permissionId)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// SetStatusAwayIfNeeded provides a mock function with given fields: userID, manual
func (_m *SuiteIFace) SetStatusAwayIfNeeded(userID string, manual bool) {
	_m.Called(userID, manual)
}

// SetStatusLastActivityAt provides a mock function with given fields: userID, activityAt
func (_m *SuiteIFace) SetStatusLastActivityAt(userID string, activityAt int64) {
	_m.Called(userID, activityAt)
}

// SetStatusOffline provides a mock function with given fields: userID, manual, updateLastActivityAt
func (_m *SuiteIFace) SetStatusOffline(userID string, manual bool, updateLastActivityAt bool) {
	_m.Called(userID, manual, updateLastActivityAt)
}

// SetStatusOnline provides a mock function with given fields: userID, manual
func (_m *SuiteIFace) SetStatusOnline(userID string, manual bool) {
	_m.Called(userID, manual)
}

// UpdateLastActivityAtIfNeeded provides a mock function with given fields: session
func (_m *SuiteIFace) UpdateLastActivityAtIfNeeded(session model.Session) {
	_m.Called(session)
}

// UserCanSeeOtherUser provides a mock function with given fields: userID, otherUserId
func (_m *SuiteIFace) UserCanSeeOtherUser(userID string, otherUserId string) (bool, *model.AppError) {
	ret := _m.Called(userID, otherUserId)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string, string) bool); ok {
		r0 = rf(userID, otherUserId)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string, string) *model.AppError); ok {
		r1 = rf(userID, otherUserId)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

type mockConstructorTestingTNewSuiteIFace interface {
	mock.TestingT
	Cleanup(func())
}

// NewSuiteIFace creates a new instance of SuiteIFace. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewSuiteIFace(t mockConstructorTestingTNewSuiteIFace) *SuiteIFace {
	mock := &SuiteIFace{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
