package helpers

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// UploadImage handles uploading a multipart file to either Cloudinary or Local Storage.
// Returns the secure URL if Cloudinary is used, or the local path if falling back.
func UploadImage(file multipart.File, header *multipart.FileHeader, affiliateID string, localDir string, prefix string) (string, error) {
	cloudinaryURL := os.Getenv("CLOUDINARY_URL")
	ext := filepath.Ext(header.Filename)

	if cloudinaryURL != "" {
		// Cloudinary Upload
		cld, err := cloudinary.NewFromURL(cloudinaryURL)
		if err != nil {
			log.Printf("Failed to initialize Cloudinary, falling back to local: %v", err)
			return uploadLocal(file, affiliateID, localDir, prefix, ext)
		}

		ctx := context.Background()
		publicID := fmt.Sprintf("%s_%s_%s_%d", prefix, affiliateID, filepath.Base(localDir), time.Now().UnixNano())

		resp, err := cld.Upload.Upload(ctx, file, uploader.UploadParams{
			PublicID: publicID,
			Folder:   "shopTemplate/" + affiliateID + "/" + localDir,
		})
		if err != nil {
			log.Printf("Failed to upload to Cloudinary, falling back to local: %v", err)
			// Reset file pointer for local upload attempt
			file.Seek(0, 0)
			return uploadLocal(file, affiliateID, localDir, prefix, ext)
		}
		
		return resp.SecureURL, nil
	}

	// Local Fallback
	return uploadLocal(file, affiliateID, localDir, prefix, ext)
}

func uploadLocal(file multipart.File, affiliateID string, localDir string, prefix string, ext string) (string, error) {
	uploadPath := filepath.Join("public", "images", affiliateID, localDir)
	if err := os.MkdirAll(uploadPath, 0755); err != nil {
		return "", err
	}

	newFileName := fmt.Sprintf("%s_%d%s", prefix, time.Now().UnixNano(), ext)
	fullPath := filepath.Join(uploadPath, newFileName)

	dst, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}

	return "/" + fullPath, nil
}
