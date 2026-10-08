/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service

import (
	"context"
	"errors"
	"testing"

	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/domain/user/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

const testIssuer = "https://vectoree.ai"

// fakeUserRepo implements only the methods EstablishSession reaches for an
// existing account; any other call panics through the nil embedded interface.
type fakeUserRepo struct {
	repository.UserRepository
	users []*model.User
}

func (r *fakeUserRepo) GetUserByVectoreeIdentity(_ context.Context, issuer, subject string) (*model.User, bool, error) {
	for _, u := range r.users {
		if u.VectoreeIssuer == issuer && u.VectoreeSubject == subject {
			return u, true, nil
		}
	}
	return nil, false, nil
}

func (r *fakeUserRepo) GetUsersByEmail(_ context.Context, email string) (*model.User, bool, error) {
	for _, u := range r.users {
		if u.Email == email {
			return u, true, nil
		}
	}
	return nil, false, nil
}

func (r *fakeUserRepo) BindVectoreeIdentity(_ context.Context, userID int64, issuer, subject string) (bool, error) {
	for _, u := range r.users {
		if u.ID == userID && u.VectoreeSubject == "" {
			u.VectoreeIssuer, u.VectoreeSubject = issuer, subject
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeUserRepo) UpdateSessionKey(_ context.Context, userID int64, sessionKey string) error {
	for _, u := range r.users {
		if u.ID == userID {
			u.SessionKey = sessionKey
		}
	}
	return nil
}

type fakeIDGen struct{ idgen.IDGenerator }

func (fakeIDGen) GenID(context.Context) (int64, error) { return 42, nil }

type fakeStorage struct{ storage.Storage }

func (fakeStorage) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) {
	return "", nil
}

func newTestDomain(users ...*model.User) (*userImpl, *fakeUserRepo) {
	repo := &fakeUserRepo{users: users}
	return &userImpl{Components: &Components{UserRepo: repo, IDGen: fakeIDGen{}, IconOSS: fakeStorage{}}}, repo
}

func identity(email, subject string, verified bool) userEntity.VectoreeIdentity {
	return userEntity.VectoreeIdentity{Issuer: testIssuer, Subject: subject, Email: email, EmailVerified: verified}
}

func requireConflict(t *testing.T, err error) {
	t.Helper()
	var status errorx.StatusError
	if !errors.As(err, &status) || status.Code() != errno.ErrVectoreeIdentityConflict {
		t.Fatalf("err = %v, want identity conflict", err)
	}
}

func TestBoundIdentitySignsInEvenAfterAnEmailChange(t *testing.T) {
	alice := &model.User{ID: 1, Email: "alice@example.com", VectoreeIssuer: testIssuer, VectoreeSubject: "vt_alice"}
	svc, _ := newTestDomain(alice)

	got, err := svc.EstablishSession(context.Background(), identity("alice@new.example.com", "vt_alice", true), "")
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != 1 || got.SessionKey == "" {
		t.Fatalf("user = %#v", got)
	}
}

func TestOtherVectoreeAccountCannotTakeABoundEmail(t *testing.T) {
	alice := &model.User{ID: 1, Email: "alice@example.com", VectoreeIssuer: testIssuer, VectoreeSubject: "vt_alice"}
	svc, _ := newTestDomain(alice)

	_, err := svc.EstablishSession(context.Background(), identity("alice@example.com", "vt_attacker", true), "")
	requireConflict(t, err)
	if alice.SessionKey != "" {
		t.Fatal("a session was issued for the victim account")
	}
}

func TestUnverifiedEmailCannotClaimAnUnboundAccount(t *testing.T) {
	admin := &model.User{ID: 1, Email: "admin@example.com"}
	svc, _ := newTestDomain(admin)

	_, err := svc.EstablishSession(context.Background(), identity("admin@example.com", "vt_someone", false), "")
	requireConflict(t, err)
	if admin.VectoreeSubject != "" || admin.SessionKey != "" {
		t.Fatalf("admin = %#v", admin)
	}
}

func TestVerifiedEmailBindsAnUnboundAccountOnce(t *testing.T) {
	admin := &model.User{ID: 1, Email: "admin@example.com"}
	svc, _ := newTestDomain(admin)

	if _, err := svc.EstablishSession(context.Background(), identity("admin@example.com", "vt_admin", true), ""); err != nil {
		t.Fatal(err)
	}
	if admin.VectoreeIssuer != testIssuer || admin.VectoreeSubject != "vt_admin" {
		t.Fatalf("admin = %#v", admin)
	}

	_, err := svc.EstablishSession(context.Background(), identity("admin@example.com", "vt_second", true), "")
	requireConflict(t, err)
}

func TestMissingVectoreeUserIDIsRejected(t *testing.T) {
	svc, _ := newTestDomain()

	_, err := svc.EstablishSession(context.Background(), identity("a@example.com", "", true), "")
	requireConflict(t, err)
}
