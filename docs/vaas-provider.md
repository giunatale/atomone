# AtomOne as a VAAS provider

This branch (`vaas-pre-mvp`) wires the VAAS provider module into AtomOne, so
AtomOne leases its validator set to consumer chains over IBC v2. It is the
provider for the VAAS testnet and the base of the pull request that
supersedes atomone-hub/atomone#283 (February 2026, `julien/wire-vaas`: keepers,
keys, modules and go.mod against allinbits/vaas#18), which the protocol has
moved past. This page is the handover: what the branch changes, why, how it
was verified, and what turning it into a pull request takes.

Branch base: atomone-hub/atomone `main` at c50c7b09 ("fix(amino)!: align
LegacyAmino registrations with the proto").

## Dependency pins

- `github.com/cosmos/cosmos-sdk` and `cosmossdk.io/x/evidence` are replaced
  by atomone-sdk at `main` e606aa2 (2026-11-12). AtomOne `main` pins the
  v0.500.7 release line (277cc1fb), which lacks what VAAS depends on: the
  pre-rotation staking hook (atomone-sdk #137), x/evidence ignoring evidence
  for unknown validators (#138), the genesis round trip of consensus-key
  rotation state (#140), the openpgp armor replacement (#157) and the
  dependency bumps for the published advisories (#158). The x/mint rewrite
  that follows on atomone-sdk `main` is not taken. The release-line question
  (a tag that carries these) is open with the atomone-sdk maintainers; the
  mainnet path is a tag, not a branch.
- `github.com/allinbits/vaas` is replaced by giunatale/vaas at the `pre-mvp`
  integration branch (every open VAAS pull request merged in release order)
  until VAAS tags a release. To move it:

  ```bash
  go list -m -json github.com/giunatale/vaas@<sha> | jq -r .Version   # the pseudo-version
  go mod edit -replace github.com/allinbits/vaas=github.com/giunatale/vaas@<pseudo-version>
  go mod tidy
  ```

  `go get github.com/allinbits/vaas@<sha>` resolves against allinbits and
  fails for a commit that only exists on the fork.
- atomone-sdk e606aa2 adds `BeforeConsensusPubKeyRotation` to the staking
  hooks interface; `x/coredaos`'s hooks answer it with a no-op
  (`x/coredaos/keeper/hooks.go`).

## The wiring

Everything is in `app/keepers/keepers.go`, `app/keepers/keys.go`,
`app/keepers/vaas_photon.go` and `app/modules.go`; `app/vaas_wiring_test.go`
pins each point.

1. Store key `vaasprovider` (`providertypes.StoreKey`) in `GenerateKeys`.
2. `ProviderKeeper` built after the gov keeper with: the IBC client, client
   v2 and channel v2 keepers; the staking and slashing keepers; the photon
   rate adapter; the account, bank and distribution keepers; the gov
   authority; the validator and consensus address codecs; the fee collector
   name; `uphoton` as the fee denom.
3. `SetGovKeeper(govadapter.New(GovKeeper))` right after, since the gov
   keeper is a pointer that is still being completed: the provider defers
   punishments behind a consumer's removal vote through it.
4. `BankKeeper.AppendSendRestriction(ProviderKeeper.FeePoolSendRestriction())`:
   the per-consumer fee pools accept deposits only through the module's
   messages.
5. `ProviderKeeper.Hooks()` appended to the staking hooks, after coredaos's.
6. IBC v2 route `vaasprovider` to `ibcprovider.NewIBCModule(&ProviderKeeper)`
   on the v2 router, beside transfer.
7. Module account `vaasprovider` with the Burner permission.
8. `no_valupdates_genutil` and `no_valupdates_staking` from VAAS in place of
   cosmos-sdk's genutil and staking modules: the provider module hands
   CometBFT the validator set from its EndBlock, so neither pushes its own.
9. `ibcprovider.NewAppModule(&ProviderKeeper)` in the module list.
10. `vaasprovider` last in the begin-block, end-block and init-genesis orders.
11. `app/keepers/vaas_photon.go`: `VAASPhotonRate` presents x/photon's
    conversion rate (`PhotonConversionRate(ctx, bondSupply, photonSupply)`)
    as the `ConversionRate(ctx)` the provider's photon keeper interface
    asks for.
12. `Makefile`: `MIN_VOTING_PERIOD` also sets cosmos-sdk's
    `x/gov/types/v1.MinVotingPeriod` beside AtomOne's own, because
    atomone-sdk's gov module validates the voting period at genesis
    validation and on parameter updates; a test network needs both floors
    lowered.

The ante handler, the fee market (`x/dynamicfee`) and `x/photon` are
untouched: provider transactions pay `uphoton` fees like every other
AtomOne transaction.

## Verification done on this branch

- `go build ./...`, `go vet ./...`, `go test ./app/... ./x/...`.
- `atomoned init` plus `genesis validate` with the `vaasprovider` section.
- A single-validator run produces blocks with the provider's EndBlock
  computing the validator set and serves `query vaasprovider params`.
- A localnet with the VAAS reference consumer: AtomOne provider (1 s commits,
  `blocks_per_epoch` 10, gov 300 s with the companion fields, dynamicfee
  `fee_denom` set to `uphoton`, `key_rotation_fee` in `uatone`), the
  consumer from the vaas repository's `consumer-init`, `consumer-genesis` and
  `consumer-run` targets, the IBC v2 ts-relayer with a `0.025uphoton` gas
  price on the provider; `create-consumer`, both client declarations,
  `fund-consumer-fee-pool` and a delegation all land; the consumer receives
  the VSC packets and its CometBFT validator set reflects the delegation.

Genesis values a test network sets on top of `init`: `dynamicfee.params.fee_denom`
to `uphoton` (the default is `stake`), `staking.params.key_rotation_fee` in
`uatone`, the gov voting period with `quorum_timeout`,
`max_voting_period_extension` and both deposit throttlers' `update_period`
consistent with it, and `minimum-gas-prices` in `app.toml` naming both
`uatone` and `uphoton`.

## Known items

- The fee-pool send restriction reads the store once per bank output, 1000
  gas each. The e2e dynamicfee test's multisend used 4200 outputs, which
  pushed its estimate past the e2e genesis block cap of 60M gas; it now sends
  3500 (about 35M gas, still above the dynamicfee target of half the block).
- The deprecations atomone-sdk e606aa2 marks are answered: `x/photon`
  asserts `appmodule.AppModule`, the genesis command uses genutil's
  `Commands`, and `app/modules.go` and the v4 queue migration keep the
  deprecated types they must, with a `nolint` each. `make lint` is clean.
- The VAAS `replace` goes away once allinbits/vaas tags a release and
  AtomOne requires it by version.

## Toward the pull request

The pull request is atomone-hub/atomone#393 (branch `feat/vaas-provider`,
draft), which supersedes #283. It carries this wiring against
allinbits/vaas `main` instead of the integrated `pre-mvp` tree this branch
pins, plus the deprecation answers, the e2e test change and a changelog
entry. What remains before it leaves draft:

1. Settle the atomone-sdk pin: a release tag carrying the five commits above,
   or `main` for the testnet only.
2. Replace the VAAS pseudo-version with the MVP release tag.
3. If the branch targets a network upgrade, add the provider store to that
   upgrade's `StoreUpgrades` and a note in `UPGRADING.md`: no upgrade adds it
   yet, and the testnet starts from a fresh genesis.

This branch stays the integration target for the test networks: rebase it
onto `main` when AtomOne moves, and re-pin VAAS when `pre-mvp` moves.
