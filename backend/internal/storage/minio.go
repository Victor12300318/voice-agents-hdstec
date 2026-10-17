// Package storage gerencia a persistência de gravações WAV (em volume local /data/recordings e/ou S3 MinIO).
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage struct {
	LocalDir string
	Client   *minio.Client
	Bucket   string
	Log      *slog.Logger
}

func New(ctx context.Context, endpoint, accessKey, secretKey, bucket string, log *slog.Logger) (*Storage, error) {
	if bucket == "" {
		bucket = "recordings"
	}
	if log == nil {
		log = slog.Default()
	}

	localDir := os.Getenv("RECORDINGS_DIR")
	if localDir == "" {
		localDir = "/data/recordings"
	}
	_ = os.MkdirAll(localDir, 0o755)

	st := &Storage{
		LocalDir: localDir,
		Bucket:   bucket,
		Log:      log,
	}

	// Se um endpoint S3/MinIO externo estiver configurado e acessível, inicializa também o cliente S3
	if endpoint != "" && accessKey != "" && secretKey != "" {
		cli, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: false,
		})
		if err == nil {
			exists, bErr := cli.BucketExists(ctx, bucket)
			if bErr == nil {
				if !exists {
					_ = cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
				}
				st.Client = cli
			}
		}
	}

	return st, nil
}

func (s *Storage) localPath(key string) string {
	clean := filepath.Clean(strings.TrimLeft(key, "/\\"))
	return filepath.Join(s.LocalDir, clean)
}

// UploadRecording salva o áudio WAV estéreo em disco persistente (/data/recordings) e/ou no S3/MinIO.
// Chave: recordings/{organization_id}/{agent_id}/{conversation_id}/audio.wav
func (s *Storage) UploadRecording(ctx context.Context, key string, wavData []byte) error {
	if s == nil || len(wavData) == 0 {
		return nil
	}

	if s.LocalDir != "" {
		fullPath := s.localPath(key)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err == nil {
			if wErr := os.WriteFile(fullPath, wavData, 0o644); wErr == nil {
				s.Log.Info("recording_saved_local", "path", fullPath, "bytes", len(wavData))
				if s.Client == nil {
					return nil
				}
			}
		}
	}

	if s.Client != nil {
		reader := bytes.NewReader(wavData)
		_, err := s.Client.PutObject(ctx, s.Bucket, key, reader, int64(len(wavData)), minio.PutObjectOptions{
			ContentType: "audio/wav",
		})
		if err != nil {
			return fmt.Errorf("upload minio %q: %w", key, err)
		}
		s.Log.Info("recording_uploaded", "bucket", s.Bucket, "key", key, "bytes", len(wavData))
	}
	return nil
}

// GetRecording busca o stream de áudio para reprodução pelo player do navegador.
func (s *Storage) GetRecording(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if s == nil {
		return nil, 0, fmt.Errorf("armazenamento de gravações indisponível")
	}

	if s.LocalDir != "" {
		fullPath := s.localPath(key)
		if f, err := os.Open(fullPath); err == nil {
			if st, sErr := f.Stat(); sErr == nil {
				return f, st.Size(), nil
			}
			_ = f.Close()
		}
	}

	if s.Client != nil {
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

	return nil, 0, fmt.Errorf("gravação %q não encontrada", key)
}
