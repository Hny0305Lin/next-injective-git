package storageconfig

// Select is keyed by immutable context, never a mutable owner/repository name.
// Storage selection cannot change a network profile, chain ID or Directory.
func (c Config) Select(chainID, directory, repoID string) (Profile, Profile, error) {
	if c.Validate() != nil {
		return Profile{}, Profile{}, ErrConfig
	}
	for _, b := range c.Repositories {
		if b.ChainID == chainID && b.SuiteDirectory == directory && b.RepoID == repoID {
			return c.Profiles[b.Writer], c.Profiles[b.Reader], nil
		}
	}
	return Profile{}, Profile{}, ErrConfig
}
