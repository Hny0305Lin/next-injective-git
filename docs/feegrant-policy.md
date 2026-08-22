# Gas Sponsorship Status

The former chain-native feegrant wrappers are not part of the immutable EVM
Suite runtime or default CI/release path. Every current write is a legacy EVM
transaction paid by its signer with explicit estimated gas and a minimum gas
price of `160000000 wei`.

Any future user gas sponsorship must be designed as a separate EVM-compatible
service with bounded authorization, replay protection, rate limits, accounting,
and abuse controls. It must not introduce a second chain backend or bypass
SuiteDirectory verification.
