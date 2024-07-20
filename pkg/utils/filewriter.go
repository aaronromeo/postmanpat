package utils

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Writer interface {
	Write(p []byte) (n int, err error)
	Flush() error
}

type FileManager interface {
	Close() error
	Create(name string) (Writer, error)
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(filename string, data []byte, perm os.FileMode) error
	ReadFile(filename string) ([]byte, error)
}

type OSFileManager struct {
	Outfile *os.File
	Writer  Writer
}

func (osfc OSFileManager) Create(name string) (Writer, error) {
	var err error
	osfc.Outfile, err = os.Create(name)
	if err != nil {
		return nil, err
	}
	osfc.Writer = bufio.NewWriter(osfc.Outfile)
	return osfc.Writer, nil
}

func (osfc OSFileManager) Close() error {
	if err := osfc.Writer.Flush(); err != nil {
		return err
	}
	if err := osfc.Outfile.Close(); err != nil {
		return err
	}

	return nil
}

func (osfc OSFileManager) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osfc OSFileManager) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (osfc OSFileManager) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

type S3FileManager struct {
	svc    *s3.S3
	bucket string
	folder string
	writer *S3Writer
	objKey string
}

func NewS3FileManager(sess *session.Session, bucket, folder string) *S3FileManager {
	return &S3FileManager{
		svc:    s3.New(sess),
		bucket: bucket,
		folder: folder,
	}
}

func (s3fm *S3FileManager) Create(name string) (Writer, error) {
	s3fm.objKey = filepath.Join(s3fm.folder, name)
	s3fm.writer = new(S3Writer)
	return s3fm.writer, nil
}

func (s3fm *S3FileManager) Close() error {
	_, err := s3fm.svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s3fm.bucket),
		Key:    aws.String(s3fm.objKey),
		Body:   bytes.NewReader((*s3fm.writer).buffer.Bytes()),
	})
	return err
}

func (s3fm *S3FileManager) MkdirAll(path string, perm os.FileMode) error {
	_, err := s3fm.svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s3fm.bucket),
		Key:    aws.String(filepath.Join(s3fm.folder, path) + "/"),
	})
	return err
}

func (s3fm *S3FileManager) WriteFile(filename string, data []byte, perm os.FileMode) error {
	_, err := s3fm.svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s3fm.bucket),
		Key:    aws.String(filepath.Join(s3fm.folder, filename)),
		Body:   bytes.NewReader(data),
	})
	return err
}

func (s3fm *S3FileManager) ReadFile(filename string) ([]byte, error) {
	obj, err := s3fm.svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s3fm.bucket),
		Key:    aws.String(filepath.Join(s3fm.folder, filename)),
	})
	if err != nil {
		return nil, err
	}

	defer obj.Body.Close()

	data, err := io.ReadAll(obj.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s3fm *S3FileManager) BucketExists(bucket string) (bool, error) {
	result, err := s3fm.svc.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		return false, err
	}

	for _, b := range result.Buckets {
		if aws.StringValue(b.Name) == bucket {
			return true, nil
		}
	}

	return false, nil
}

func (s3fm *S3FileManager) CreateBucket(bucket string) error {
	_, err := s3fm.svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return err
	}

	// Wait until the bucket is created before finishing
	err = s3fm.svc.WaitUntilBucketExists(&s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return err
	}

	return nil
}

// S3Writer struct implementing Writer interface for S3
type S3Writer struct {
	buffer *bytes.Buffer
}

func (s3w *S3Writer) Write(p []byte) (n int, err error) {
	return s3w.buffer.Write(p)
}

func (s3w *S3Writer) Flush() error {
	return nil // No action needed for flush in this implementation
}
