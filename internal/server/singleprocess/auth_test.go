package singleprocess

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/hashicorp/waypoint/internal/server/gen"
)

func TestServiceAuth(t *testing.T) {
	// Create our server
	impl, err := New(WithDB(testDB(t)))
	require.NoError(t, err)

	t.Run("create and validate a token", func(t *testing.T) {
		s := impl.(*service)

		md := map[string]string{
			"addr": "test",
		}

		token, err := s.NewLoginToken(DefaultKeyId, md, nil)
		require.NoError(t, err)

		require.True(t, len(token) > 5)
		t.Logf("token: %s", token)

		tt, body, err := s.decodeToken(token)
		require.NoError(t, err)

		kind, ok := body.Kind.(*pb.Token_Login_)
		assert.True(t, ok)
		assert.Equal(t, DefaultUser, kind.Login.UserId)
		assert.Equal(t, md, tt.Metadata)

		err = s.Authenticate(context.Background(), token, "test", nil)
		require.NoError(t, err)

		// Now corrupt the token and check that validation fails
		data := []byte(token)

		data[len(data)-2] = data[len(data)-2] + 1
		_, _, err = s.decodeToken(string(data))
		require.Error(t, err)

		// Generate a legit token with an unknown key though
	})

	t.Run("entrypoint token can only access entrypoint APIs", func(t *testing.T) {
		s := impl.(*service)

		token, err := s.NewLoginToken(DefaultKeyId, nil, &pb.Token_Entrypoint{})
		require.NoError(t, err)

		{
			err := s.Authenticate(context.Background(), token, "EntrypointConfig", nil)
			require.NoError(t, err)
		}

		{
			err := s.Authenticate(context.Background(), token, "UpsertDeployment", nil)
			require.Error(t, err)
		}
	})

	t.Run("rejects tokens signed with unknown keys", func(t *testing.T) {
		s := impl.(*service)

		md := map[string]string{
			"addr": "test",
		}

		token, err := s.NewLoginToken(DefaultKeyId, md, nil)
		require.NoError(t, err)

		require.True(t, len(token) > 5)
		t.Logf("token: %s", token)

		tt, body, err := s.decodeToken(token)
		require.NoError(t, err)
		bodyData, err := proto.Marshal(body)
		require.NoError(t, err)

		h, err := blake2b.New256([]byte("abcdabcdabcdabcadacdacdaaa"))
		require.NoError(t, err)

		h.Write(bodyData)

		tt.Signature = h.Sum(nil)

		ttData, err := proto.Marshal(tt)
		require.NoError(t, err)

		var buf bytes.Buffer
		buf.WriteString(tokenMagic)
		buf.Write(ttData)

		rogue := base58.Encode(buf.Bytes())

		_, _, err = s.decodeToken(rogue)
		require.Error(t, err)
	})

	t.Run("exchange an invite token", func(t *testing.T) {
		s := impl.(*service)

		invite, err := s.NewInviteToken(2*time.Second, DefaultKeyId, nil, nil)
		require.NoError(t, err)

		lt, err := s.ExchangeInvite(DefaultKeyId, invite)
		require.NoError(t, err)

		_, body, err := s.decodeToken(lt)
		require.NoError(t, err)

		_, ok := body.Kind.(*pb.Token_Login_)
		assert.True(t, ok)

		time.Sleep(3 * time.Second)

		_, err = s.ExchangeInvite(DefaultKeyId, invite)
		require.Error(t, err)
	})

	t.Run("exchange an entrypoint invite token", func(t *testing.T) {
		s := impl.(*service)

		entry := &pb.Token_Entrypoint{DeploymentId: "foo"}

		invite, err := s.NewInviteToken(2*time.Second, DefaultKeyId, nil, entry)
		require.NoError(t, err)

		lt, err := s.ExchangeInvite(DefaultKeyId, invite)
		require.NoError(t, err)

		_, body, err := s.decodeToken(lt)
		require.NoError(t, err)

		kind, ok := body.Kind.(*pb.Token_Login_)
		assert.True(t, ok)
		assert.NotNil(t, kind.Login.Entrypoint)
		assert.Equal(t, entry.DeploymentId, kind.Login.Entrypoint.DeploymentId)

		time.Sleep(3 * time.Second)

		_, err = s.ExchangeInvite(DefaultKeyId, invite)
		require.Error(t, err)
	})
}

func TestServiceBootstrapToken(t *testing.T) {
	ctx := context.Background()
	require := require.New(t)

	// Create our server
	impl, err := New(WithDB(testDB(t)))
	require.NoError(err)

	{
		// Initial bootstrap should return a token
		resp, err := impl.BootstrapToken(ctx, &empty.Empty{})
		require.NoError(err)
		require.NotEmpty(resp.Token)
	}

	{
		// Subs calls should fail
		resp, err := impl.BootstrapToken(ctx, &empty.Empty{})
		require.Error(err)
		require.Equal(codes.PermissionDenied, status.Code(err))
		require.Nil(resp)
	}
}
