// Package storage gerencia a persistência de gravações no MinIO.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage struct {
	Client *minio.Client
	Bucket string
	Log    *slog.Logger
}

func New(ctx context.Context, endpoint, accessKey, secretKey, bucket string, log *slog.Logger) (*Storage, error) {
	if endpoint == "" {
		endpoint = "minio:9000"
	}
	if bucket == "" {
		bucket = "recordings"
	}
	if log == nil {
		log = slog.Default()
	}

	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false, // MinIO local no docker
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	// Garante que o bucket de gravações existe
	exists, err := cli.BucketExists(ctx, bucket)
	if err != nil {
		log.Warn("minio_bucket_check_warning", "bucket", bucket, "error", err)
	} else if !exists {
		err = cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("minio make bucket %q: %w", bucket, err)
		}
		log.Info("minio_bucket_created", "bucket", bucket)
	}

	return &Storage{Client: cli, Bucket: bucket, Log: log}, nil
}

// UploadRecording salva o áudio WAV estéreo no MinIO.
// Chave: recordings/{organization_id}/{agent_id}/{conversation_id}/audio.wav
func (s *Storage) UploadRecording(ctx context.Context, key string, wavData []byte) error {
	if s == nil || s.Client == nil || len(wavData) == 0 {
		return nil
	}
	reader := bytes.NewReader(wavData)
	_, err := s.Client.PutObject(ctx, s.Bucket, key, reader, int64(len(wavData)), minio.PutObjectOptions{
		ContentType: "audio/wav",
	})
	if err != nil {
		return fmt.Errorf("upload minio %q: %w", key, err)
	}
	s.Log.Info("recording_uploaded", "bucket", s.Bucket, "key", key, "bytes", len(wavData))
	return nil
}

// GetRecording busca o stream de áudio para reprodução pelo player do navegador.
func (s *Storage) GetRecording(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if s == nil || s.Client == nil {
		return nil, 0, fmt.Errorf("armazenamento MinIO indisponível")
	}
	obj, err := s.Client.GetObject(ctx, s.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, err
	}
	stat, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, 0, err
	}
	return obj, stat.Size, nil
}
