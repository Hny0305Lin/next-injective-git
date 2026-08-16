//go:build windows

package fileprotection

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProtectFile gives only the current user and LocalSystem full access and
// disables inherited permissions.
func ProtectFile(path string) error {
	if err := validateObjectType(path, false); err != nil {
		return err
	}
	return protect(path, windows.NO_INHERITANCE)
}

// ProtectDirectory applies the same principals and lets child objects inherit
// the protected directory policy.
func ProtectDirectory(path string) error {
	if err := validateObjectType(path, true); err != nil {
		return err
	}
	return protect(path, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
}

func protect(path string, inheritance uint32) error {
	user, system, err := allowedSIDs()
	if err != nil {
		return err
	}
	sids := []*windows.SID{user}
	if !user.Equals(system) {
		sids = append(sids, system)
	}
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(sids))
	for _, sid := range sids {
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       inheritance,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("build sensitive-file DACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		user,
		nil,
		acl,
		nil,
	); err != nil {
		return fmt.Errorf("set sensitive-file DACL: %w", err)
	}
	return nil
}

func replaceFile(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// ValidateFile verifies that the DACL is protected and grants full access to
// no principals other than the current user and LocalSystem.
func ValidateFile(path string) error {
	return validate(path, false)
}

// ValidateDirectory also verifies that its two access entries propagate to
// child files and directories.
func ValidateDirectory(path string) error {
	return validate(path, true)
}

func validate(path string, directory bool) error {
	if err := validateObjectType(path, directory); err != nil {
		return err
	}
	user, system, err := allowedSIDs()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read sensitive-file DACL: %w", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read sensitive-file owner: %w", err)
	}
	if owner == nil || (!owner.Equals(user) && !owner.Equals(system)) {
		return fmt.Errorf("sensitive-file owner is not the current user or LocalSystem")
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return fmt.Errorf("read sensitive-file DACL control: %w", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("sensitive-file DACL inherits permissions")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read sensitive-file access list: %w", err)
	}
	if dacl == nil || dacl.AceCount == 0 {
		return fmt.Errorf("sensitive-file DACL has no access entries")
	}
	type grantState struct {
		access   bool
		children bool
	}
	want := map[string]grantState{user.String(): {}, system.String(): {}}
	if user.Equals(system) {
		delete(want, system.String())
		want[user.String()] = grantState{}
	}
	const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return fmt.Errorf("read sensitive-file DACL entry %d: %w", index, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("sensitive-file DACL entry %d is not an allow entry", index)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		key := sid.String()
		state, ok := want[key]
		if !ok {
			return fmt.Errorf("sensitive-file DACL grants access to unexpected SID %s", key)
		}
		mask := uint32(ace.Mask)
		if mask&uint32(windows.GENERIC_ALL) == 0 && mask&uint32(fileAllAccess) != uint32(fileAllAccess) {
			return fmt.Errorf("sensitive-file DACL does not grant full access to %s", key)
		}
		if uint32(ace.Header.AceFlags)&uint32(windows.INHERIT_ONLY_ACE) == 0 {
			state.access = true
		}
		const childInheritance = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
		if uint32(ace.Header.AceFlags)&uint32(childInheritance) == uint32(childInheritance) {
			state.children = true
		}
		want[key] = state
	}
	for sid, state := range want {
		if !state.access {
			return fmt.Errorf("sensitive-file DACL is missing SID %s", sid)
		}
		if directory && !state.children {
			return fmt.Errorf("sensitive-directory DACL does not propagate SID %s to children", sid)
		}
	}
	return nil
}

func validateObjectType(path string, directory bool) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(pathPtr)
	if err != nil {
		return err
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("sensitive path is a Windows reparse point")
	}
	isDirectory := attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if directory && !isDirectory {
		return fmt.Errorf("sensitive directory is not a directory")
	}
	if !directory && isDirectory {
		return fmt.Errorf("sensitive file is not a regular file")
	}
	return nil
}

func allowedSIDs() (*windows.SID, *windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve current Windows user SID: %w", err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve Windows LocalSystem SID: %w", err)
	}
	return user.User.Sid, system, nil
}
