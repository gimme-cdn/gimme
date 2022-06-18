package content

import (
	"archive/zip"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/gimme-cdn/gimme/internal/storage"

	"github.com/gimme-cdn/gimme/internal/errors"
	"golang.org/x/mod/semver"
)

type ContentService struct {
	objectStorageManager storage.ObjectStorageManager
}

type File struct {
	Name   string
	Size   int64
	Folder bool
}

var re = regexp.MustCompile(`^[a-zA-Z0-9-_]+`)

// NewContentService create a new content service instance
func NewContentService(objectStorageManager storage.ObjectStorageManager) ContentService {
	return ContentService{
		objectStorageManager,
	}
}

// filterArray filter objects array
func (svc *ContentService) filterArray(arr []*s3.Object, fileName string, version string) []*s3.Object {
	var filtered []*s3.Object
	for _, item := range arr {
		if strings.Contains(*item.Key, fileName) && strings.Contains(*item.Key, version) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// filterArray get package version
func (svc *ContentService) getVersion(objStorageFile string) string {
	return strings.Split(strings.Split(objStorageFile, "@")[1], "/")[0]
}

// getLatestVersion get last package version
func (svc *ContentService) getLatestVersion(arr []*s3.Object) string {
	var versions []string
	for _, curr := range arr {
		versions = append(versions, svc.getVersion(*curr.Key))
	}
	semver.Sort(versions)
	return versions[len(versions)-1]
}

// getLatestPackagePath get latest package path
func (svc *ContentService) getLatestPackagePath(pkg string, version string, fileName string) (string, *errors.GimmeError) {
	objs, err := svc.objectStorageManager.ListObjects(fmt.Sprintf("%s@%s", pkg, version))
	if err != nil {
		return "", err
	}

	filtred := svc.filterArray(objs, fileName, version)

	if len(filtred) == 0 {
		return fmt.Sprintf("%s@%s%s", pkg, version, fileName), nil
	}

	lversion := svc.getLatestVersion(filtred)
	return fmt.Sprintf("%s@%s%s", pkg, lversion, fileName), nil
}

// CreatePackage create package
func (svc *ContentService) CreatePackage(name string, version string, file io.ReaderAt, fileSize int64) *errors.GimmeError {
	archive, err := zip.NewReader(file, fileSize)
	if err != nil {
		logrus.Error("[UploadManager] ArchiveProcessor - Error while reading zip file", err)
		return errors.NewBusinessError(errors.InternalError, fmt.Errorf("error while reading zip file"))
	}

	folderName := fmt.Sprintf("%s@%s", name, version)

	if exists, _ := svc.objectStorageManager.ObjectExists(folderName); exists {
		return errors.NewBusinessError(errors.Conflict, fmt.Errorf("the package %v already exists", folderName))
	}

	nbFiles := len(archive.File)

	var wg sync.WaitGroup
	wg.Add(nbFiles)

	for _, currentFile := range archive.File {
		go func(currentFile *zip.File) {
			defer wg.Done()
			logrus.Debug("[UploadManager] ArchiveProcessor - Unzipping file ", currentFile.Name)
			fileName := re.ReplaceAllString(currentFile.FileHeader.Name, folderName)
			err := svc.objectStorageManager.AddObject(fileName, currentFile)
			if err != nil {
				logrus.Errorf("[UploadManager] ArchiveProcessor - Error while processing file %s", fileName)
			}
		}(currentFile)
	}

	wg.Wait()
	return nil
}

// GetFile get package file
func (svc *ContentService) GetFile(pkg string, version string, fileName string) (*s3.GetObjectOutput, *errors.GimmeError) {
	valid := semver.IsValid(fmt.Sprintf("v%v", version))
	if !valid {
		return nil, errors.NewBusinessError(errors.BadRequest, fmt.Errorf("invalid version (asked version must be semver compatible)"))
	}

	var objectPath string
	slice := strings.Split(version, ".")
	if len(slice) == 3 {
		objectPath = fmt.Sprintf("%s@%s%s", pkg, version, fileName)
	} else {
		objectPath, _ = svc.getLatestPackagePath(pkg, version, fileName)
		//if err != nil {
		//	return nil, err
		//}
	}

	return svc.objectStorageManager.GetObject(objectPath)
}

// GetFiles get package files
func (svc *ContentService) GetFiles(pkg string, version string) ([]File, *errors.GimmeError) {
	objs, err := svc.objectStorageManager.ListObjects(fmt.Sprintf("%s@%s", pkg, version))

	if err != nil {
		return nil, err
	}

	var files []File
	for _, obj := range objs {
		files = append(files, File{
			Name:   *obj.Key,
			Size:   *obj.Size,
			Folder: false,
		})
	}
	return files, nil
}

// DeletePackage delete package
func (svc *ContentService) DeletePackage(pkg string, version string) *errors.GimmeError {
	//err := svc.objectStorageManager.RemoveObjects(fmt.Sprintf("%s@%s", pkg, version))
	//if err != nil {
	//	return err
	//}
	return nil
}
