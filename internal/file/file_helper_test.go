package file_test

import (
	"fmt"
	"testing"

	"github.com/konveyor/crane/internal/file"
)

func TestGetWhiteOutFilePath(t *testing.T) {
	cases := []struct {
		Name        string
		Filepath    string
		Dir         string
		ResourceDir string
		Expected    string
		WantErr     bool
	}{
		{
			Name:        "test whiteout file creation",
			Filepath:    "/fully/qualified/resources/ns/path-test",
			Dir:         "/fully/qualified/transform",
			ResourceDir: "/fully/qualified/resources",
			Expected:    "/fully/qualified/transform/ns/.wh.path-test",
			WantErr:     false,
		},
		{
			Name:        "test invalid file path; path to a directory",
			Filepath:    "/fully/qualified/resources/ns/path-test/",
			Dir:         "/fully/qualified/transform/",
			ResourceDir: "/fully/qualified/resources/ns/path-test/",
			Expected:    "",
			WantErr:     true,
		},
		{
			Name:        "nested subdirectory",
			Filepath:    "/base/resources/ns/subdir/myfile.yaml",
			Dir:         "/base/transform",
			ResourceDir: "/base/resources",
			Expected:    "/base/transform/ns/subdir/.wh.myfile.yaml",
			WantErr:     false,
		},
		{
			Name:        "file name has spaces",
			Filepath:    "/fully/qualified/resources/ns/my file.yaml",
			Dir:         "/fully/qualified/transform",
			ResourceDir: "/fully/qualified/resources",
			Expected:    "/fully/qualified/transform/ns/.wh.my file.yaml",
			WantErr:     false,
		},
		{
			Name:        "duplicated file name with space",
			Filepath:    "/fully/qualified/resources/ns/myfile (1).yaml",
			Dir:         "/fully/qualified/transform",
			ResourceDir: "/fully/qualified/resources",
			Expected:    "/fully/qualified/transform/ns/.wh.myfile (1).yaml",
			WantErr:     false,
		},
		{
			Name:        "resource dir dont match the file path",
			Filepath:    "/fully/qualified/resources/ns/myfile.yaml",
			Dir:         "/fully/qualified/transform",
			ResourceDir: "/fully/wrong/path/resources",
			Expected:    "/fully/qualified/transform/ns/.wh.myfile.yaml",
			WantErr:     true,
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			if test.WantErr {
				t.Skip("GetWhiteOutFilePath() dont handle errors yet.")
			}
			opts := file.PathOpts{
				TransformDir: test.Dir,
				ExportDir:    test.ResourceDir,
			}
			if actual := opts.GetWhiteOutFilePath(test.Filepath); actual != test.Expected {
				t.Errorf("actual: %v did not match expected: %v", actual, test.Expected)
			}
		})

	}
}

func TestGetTransformPath(t *testing.T) {
	cases := []struct {
		Name        string
		Filepath    string
		Dir         string
		ResourceDir string
		Expected    string
	}{
		{
			Name:        "test transform file creation",
			Filepath:    "/fully/qualified/ns/path-test",
			Dir:         "/fully/qualified/transform",
			ResourceDir: "/fully/qualified",
			Expected:    "/fully/qualified/transform/ns/transform-path-test",
		},
	}
	for _, test := range cases {
		opts := file.PathOpts{
			TransformDir: test.Dir,
			ExportDir:    test.ResourceDir,
		}
		if actual := opts.GetTransformPath(test.Filepath); actual != test.Expected {
			t.Errorf("actual: %v did not match expected: %v", actual, test.Expected)
		}
	}

}

func TestGetOutputFilePath(t *testing.T) {
	cases := []struct {
		Name        string
		Filepath    string
		Dir         string
		ResourceDir string
		Expected    string
	}{
		{
			Name:        "test transform file creation",
			Filepath:    "/fully/qualified/ns/path-test",
			Dir:         "/fully/qualified/output",
			ResourceDir: "/fully/qualified",
			Expected:    "/fully/qualified/output/ns/path-test",
		},
	}
	for _, test := range cases {
		opts := file.PathOpts{
			OutputDir: test.Dir,
			ExportDir: test.ResourceDir,
		}
		res := opts.GetIgnoredPatchesPath(test.Filepath)
		fmt.Printf("ignored patchespath are: %v", res)
		if actual := opts.GetOutputFilePath(test.Filepath); actual != test.Expected {
			t.Errorf("actual: %v did not match expected: %v", actual, test.Expected)
		}
	}
}

func TestGetIgnoredPatchesPath(t *testing.T) {
	tests := []struct {
		Name              string
		Filepath          string
		ResourceDir       string
		IgnoredPatchesDir string
		Expected          string
	}{
		{
			Name:              "test ignored patchs",
			Filepath:          "/fully/qualified/resources/ns/myfile.yaml",
			ResourceDir:       "/fully/qualified/transform",
			IgnoredPatchesDir: "/fully/qualified/ignored",
			Expected:          "/fully/qualified/resources/ns/ignored-myfile.yaml",
		},
		{
			Name:        "test empty ignored patchs dir",
			Filepath:    "/fully/qualified/resources/ns/myfile.yaml",
			ResourceDir: "/fully/qualified/resources",
			Expected:    "",
		},
		{
			Name:              "nested file",
			Filepath:          "/fully/qualified/resources/ns/subdir/myfile.yaml",
			ResourceDir:       "/fully/qualified/resources",
			IgnoredPatchesDir: "/fully/qualified/ignored",
			Expected:          "/fully/qualified/ignored/ns/subdir/ignored-myfile.yaml",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			opt := file.PathOpts{
				ExportDir:         test.ResourceDir,
				IgnoredPatchesDir: test.IgnoredPatchesDir,
			}
			res := opt.GetIgnoredPatchesPath(test.Filepath)
			if res != test.Expected {
				t.Errorf("result didnt match expected..\n res: %v \n expected %v \n", res, test.Expected)
			}
		})

	}
}
