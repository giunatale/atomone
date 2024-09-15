package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	v3 "github.com/atomone-hub/atomone/x/gov/migrations/v3"
	"github.com/atomone-hub/atomone/x/gov/types"
	v1 "github.com/atomone-hub/atomone/x/gov/types/v1"
	"github.com/atomone-hub/atomone/x/gov/types/v1beta1"
)

var _ v1.QueryServer = Keeper{}

// Proposal returns proposal details based on ProposalID
func (q Keeper) Proposal(c context.Context, req *v1.QueryProposalRequest) (*v1.QueryProposalResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.ProposalId == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposal id can not be 0")
	}

	ctx := sdk.UnwrapSDKContext(c)

	proposal, found := q.GetProposal(ctx, req.ProposalId)
	if !found {
		return nil, status.Errorf(codes.NotFound, "proposal %d doesn't exist", req.ProposalId)
	}

	return &v1.QueryProposalResponse{Proposal: &proposal}, nil
}

// Proposals implements the Query/Proposals gRPC method
func (q Keeper) Proposals(c context.Context, req *v1.QueryProposalsRequest) (*v1.QueryProposalsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.storeKey)
	proposalStore := prefix.NewStore(store, types.ProposalsKeyPrefix)

	filteredProposals, pageRes, err := query.GenericFilteredPaginate(
		q.cdc,
		proposalStore,
		req.Pagination,
		func(key []byte, p *v1.Proposal) (*v1.Proposal, error) {
			matchVoter, matchDepositor, matchStatus := true, true, true

			// match status (if supplied/valid)
			if v1.ValidProposalStatus(req.ProposalStatus) {
				matchStatus = p.Status == req.ProposalStatus
			}

			// match voter address (if supplied)
			if len(req.Voter) > 0 {
				voter, err := sdk.AccAddressFromBech32(req.Voter)
				if err != nil {
					return nil, err
				}

				_, matchVoter = q.GetVote(ctx, p.Id, voter)
			}

			// match depositor (if supplied)
			if len(req.Depositor) > 0 {
				depositor, err := sdk.AccAddressFromBech32(req.Depositor)
				if err != nil {
					return nil, err
				}
				_, matchDepositor = q.GetDeposit(ctx, p.Id, depositor)
			}

			if matchVoter && matchDepositor && matchStatus {
				return p, nil
			}

			return nil, nil
		}, func() *v1.Proposal {
			return &v1.Proposal{}
		})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &v1.QueryProposalsResponse{Proposals: filteredProposals, Pagination: pageRes}, nil
}

// Vote returns Voted information based on proposalID, voterAddr
func (q Keeper) Vote(c context.Context, req *v1.QueryVoteRequest) (*v1.QueryVoteResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.ProposalId == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposal id can not be 0")
	}

	if req.Voter == "" {
		return nil, status.Error(codes.InvalidArgument, "empty voter address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	voter, err := sdk.AccAddressFromBech32(req.Voter)
	if err != nil {
		return nil, err
	}
	vote, found := q.GetVote(ctx, req.ProposalId, voter)
	if !found {
		return nil, status.Errorf(codes.InvalidArgument,
			"voter: %v not found for proposal: %v", req.Voter, req.ProposalId)
	}

	return &v1.QueryVoteResponse{Vote: &vote}, nil
}

// Votes returns single proposal's votes
func (q Keeper) Votes(c context.Context, req *v1.QueryVotesRequest) (*v1.QueryVotesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.ProposalId == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposal id can not be 0")
	}

	var votes v1.Votes
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.storeKey)
	votesStore := prefix.NewStore(store, types.VotesKey(req.ProposalId))

	pageRes, err := query.Paginate(votesStore, req.Pagination, func(key []byte, value []byte) error {
		var vote v1.Vote
		if err := q.cdc.Unmarshal(value, &vote); err != nil {
			return err
		}

		votes = append(votes, &vote)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &v1.QueryVotesResponse{Votes: votes, Pagination: pageRes}, nil
}

// Params queries all params
func (q Keeper) Params(c context.Context, req *v1.QueryParamsRequest) (*v1.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	ctx := sdk.UnwrapSDKContext(c)
	params := q.GetParams(ctx)

	response := &v1.QueryParamsResponse{}

	//nolint:staticcheck
	switch req.ParamsType {
	case v1.ParamDeposit:
		depositParams := v1.NewDepositParams(params.MinDeposit, params.MaxDepositPeriod)
		response.DepositParams = &depositParams

	case v1.ParamVoting:
		votingParams := v1.NewVotingParams(params.VotingPeriod)
		response.VotingParams = &votingParams

	case v1.ParamTallying:
		tallyParams := v1.NewTallyParams(params.Quorum, params.Threshold, params.VetoThreshold)
		response.TallyParams = &tallyParams

	default:
		return nil, status.Errorf(codes.InvalidArgument,
			"%s is not a valid parameter type", req.ParamsType)

	}
	response.Params = &params

	return response, nil
}

// Deposit queries single deposit information based on proposalID, depositAddr.
func (q Keeper) Deposit(c context.Context, req *v1.QueryDepositRequest) (*v1.QueryDepositResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.ProposalId == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposal id can not be 0")
	}

	if req.Depositor == "" {
		return nil, status.Error(codes.InvalidArgument, "empty depositor address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	depositor, err := sdk.AccAddressFromBech32(req.Depositor)
	if err != nil {
		return nil, err
	}
	deposit, found := q.GetDeposit(ctx, req.ProposalId, depositor)
	if !found {
		return nil, status.Errorf(codes.InvalidArgument,
			"depositer: %v not found for proposal: %v", req.Depositor, req.ProposalId)
	}

	return &v1.QueryDepositResponse{Deposit: &deposit}, nil
}

// Deposits returns single proposal's all deposits
func (q Keeper) Deposits(c context.Context, req *v1.QueryDepositsRequest) (*v1.QueryDepositsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.ProposalId == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposal id can not be 0")
	}

	var deposits []*v1.Deposit
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.storeKey)
	depositStore := prefix.NewStore(store, types.DepositsKey(req.ProposalId))

	pageRes, err := query.Paginate(depositStore, req.Pagination, func(key []byte, value []byte) error {
		var deposit v1.Deposit
		if err := q.cdc.Unmarshal(value, &deposit); err != nil {
			return err
		}

		deposits = append(deposits, &deposit)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &v1.QueryDepositsResponse{Deposits: deposits, Pagination: pageRes}, nil
}

// TallyResult queries the tally of a proposal vote
func (q Keeper) TallyResult(c context.Context, req *v1.QueryTallyResultRequest) (*v1.QueryTallyResultResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.ProposalId == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposal id can not be 0")
	}

	ctx := sdk.UnwrapSDKContext(c)

	proposal, ok := q.GetProposal(ctx, req.ProposalId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "proposal %d doesn't exist", req.ProposalId)
	}

	var tallyResult v1.TallyResult

	switch {
	case proposal.Status == v1.StatusDepositPeriod:
		tallyResult = v1.EmptyTallyResult()

	case proposal.Status == v1.StatusPassed || proposal.Status == v1.StatusRejected || proposal.Status == v1.StatusFailed:
		tallyResult = *proposal.FinalTallyResult

	default:
		// proposal is in voting period
		_, _, tallyResult = q.Tally(ctx, proposal)
	}

	return &v1.QueryTallyResultResponse{Tally: &tallyResult}, nil
}

// Governor queries governor information based on governor address.
func (q Keeper) Governor(c context.Context, req *v1.QueryGovernorRequest) (*v1.QueryGovernorResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.GovernorAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "empty governor address")
	}

	governorAddr, err := types.GovernorAddressFromBech32(req.GovernorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	ctx := sdk.UnwrapSDKContext(c)

	governor, found := q.GetGovernor(ctx, governorAddr)
	if !found {
		return nil, status.Errorf(codes.NotFound, "governor %s doesn't exist", req.GovernorAddress)
	}

	return &v1.QueryGovernorResponse{Governor: &governor}, nil
}

// Governors queries all governors.
func (q Keeper) Governors(c context.Context, req *v1.QueryGovernorsRequest) (*v1.QueryGovernorsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.storeKey)
	governorStore := prefix.NewStore(store, types.GovernorKeyPrefix)

	var governors v1.Governors
	pageRes, err := query.Paginate(governorStore, req.Pagination, func(key []byte, value []byte) error {
		var governor v1.Governor
		if err := q.cdc.Unmarshal(value, &governor); err != nil {
			return err
		}

		governors = append(governors, &governor)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &v1.QueryGovernorsResponse{Governors: governors, Pagination: pageRes}, nil
}

// GovernanceDelegations queries all delegations of a governor.
func (q Keeper) GovernanceDelegations(c context.Context, req *v1.QueryGovernanceDelegationsRequest) (*v1.QueryGovernanceDelegationsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.GovernorAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "empty governor address")
	}

	governorAddr, err := types.GovernorAddressFromBech32(req.GovernorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.storeKey)
	delegationStore := prefix.NewStore(store, types.GovernanceDelegationsByGovernorKey(governorAddr, []byte{}))

	var delegations []*v1.GovernanceDelegation
	pageRes, err := query.Paginate(delegationStore, req.Pagination, func(key []byte, value []byte) error {
		var delegation v1.GovernanceDelegation
		if err := q.cdc.Unmarshal(value, &delegation); err != nil {
			return err
		}

		delegations = append(delegations, &delegation)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &v1.QueryGovernanceDelegationsResponse{Delegations: delegations, Pagination: pageRes}, nil
}

// GovernanceDelegation queries a delegation
func (q Keeper) GovernanceDelegation(c context.Context, req *v1.QueryGovernanceDelegationRequest) (*v1.QueryGovernanceDelegationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.DelegatorAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "empty delegator address")
	}

	delegatorAddr, err := sdk.AccAddressFromBech32(req.DelegatorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	ctx := sdk.UnwrapSDKContext(c)

	delegation, found := q.GetGovernanceDelegation(ctx, delegatorAddr)
	if !found {
		return nil, status.Errorf(codes.NotFound, "governance delegation for %s does not exist", req.DelegatorAddress)
	}

	return &v1.QueryGovernanceDelegationResponse{GovernorAddress: delegation.GovernorAddress}, nil
}

// GovernorValShares queries all validator shares of a governor.
func (q Keeper) GovernorValShares(c context.Context, req *v1.QueryGovernorValSharesRequest) (*v1.QueryGovernorValSharesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	if req.GovernorAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "empty governor address")
	}

	governorAddr, err := types.GovernorAddressFromBech32(req.GovernorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.storeKey)
	valShareStore := prefix.NewStore(store, types.ValidatorSharesByGovernorKey(governorAddr, []byte{}))

	var valShares []*v1.GovernorValShares
	pageRes, err := query.Paginate(valShareStore, req.Pagination, func(key []byte, value []byte) error {
		var valShare v1.GovernorValShares
		if err := q.cdc.Unmarshal(value, &valShare); err != nil {
			return err
		}

		valShares = append(valShares, &valShare)

		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &v1.QueryGovernorValSharesResponse{ValShares: valShares, Pagination: pageRes}, nil
}

var _ v1beta1.QueryServer = legacyQueryServer{}

type legacyQueryServer struct {
	keeper *Keeper
}

// NewLegacyQueryServer returns an implementation of the v1beta1 legacy QueryServer interface.
func NewLegacyQueryServer(k *Keeper) v1beta1.QueryServer {
	return &legacyQueryServer{keeper: k}
}

func (q legacyQueryServer) Proposal(c context.Context, req *v1beta1.QueryProposalRequest) (*v1beta1.QueryProposalResponse, error) {
	resp, err := q.keeper.Proposal(c, &v1.QueryProposalRequest{
		ProposalId: req.ProposalId,
	})
	if err != nil {
		return nil, err
	}

	proposal, err := v3.ConvertToLegacyProposal(*resp.Proposal)
	if err != nil {
		return nil, err
	}

	return &v1beta1.QueryProposalResponse{Proposal: proposal}, nil
}

func (q legacyQueryServer) Proposals(c context.Context, req *v1beta1.QueryProposalsRequest) (*v1beta1.QueryProposalsResponse, error) {
	resp, err := q.keeper.Proposals(c, &v1.QueryProposalsRequest{
		ProposalStatus: v1.ProposalStatus(req.ProposalStatus),
		Voter:          req.Voter,
		Depositor:      req.Depositor,
		Pagination:     req.Pagination,
	})
	if err != nil {
		return nil, err
	}

	legacyProposals := make([]v1beta1.Proposal, len(resp.Proposals))
	for idx, proposal := range resp.Proposals {
		legacyProposals[idx], err = v3.ConvertToLegacyProposal(*proposal)
		if err != nil {
			return nil, err
		}
	}

	return &v1beta1.QueryProposalsResponse{
		Proposals:  legacyProposals,
		Pagination: resp.Pagination,
	}, nil
}

func (q legacyQueryServer) Vote(c context.Context, req *v1beta1.QueryVoteRequest) (*v1beta1.QueryVoteResponse, error) {
	resp, err := q.keeper.Vote(c, &v1.QueryVoteRequest{
		ProposalId: req.ProposalId,
		Voter:      req.Voter,
	})
	if err != nil {
		return nil, err
	}

	vote, err := v3.ConvertToLegacyVote(*resp.Vote)
	if err != nil {
		return nil, err
	}

	return &v1beta1.QueryVoteResponse{Vote: vote}, nil
}

func (q legacyQueryServer) Votes(c context.Context, req *v1beta1.QueryVotesRequest) (*v1beta1.QueryVotesResponse, error) {
	resp, err := q.keeper.Votes(c, &v1.QueryVotesRequest{
		ProposalId: req.ProposalId,
		Pagination: req.Pagination,
	})
	if err != nil {
		return nil, err
	}

	votes := make([]v1beta1.Vote, len(resp.Votes))
	for i, v := range resp.Votes {
		votes[i], err = v3.ConvertToLegacyVote(*v)
		if err != nil {
			return nil, err
		}
	}

	return &v1beta1.QueryVotesResponse{
		Votes:      votes,
		Pagination: resp.Pagination,
	}, nil
}

//nolint:staticcheck
func (q legacyQueryServer) Params(c context.Context, req *v1beta1.QueryParamsRequest) (*v1beta1.QueryParamsResponse, error) {
	resp, err := q.keeper.Params(c, &v1.QueryParamsRequest{
		ParamsType: req.ParamsType,
	})
	if err != nil {
		return nil, err
	}

	response := &v1beta1.QueryParamsResponse{}

	if resp.DepositParams != nil {
		minDeposit := sdk.NewCoins(resp.DepositParams.MinDeposit...)
		response.DepositParams = v1beta1.NewDepositParams(minDeposit, *resp.DepositParams.MaxDepositPeriod)
	}

	if resp.VotingParams != nil {
		response.VotingParams = v1beta1.NewVotingParams(*resp.VotingParams.VotingPeriod)
	}

	if resp.TallyParams != nil {
		quorum, err := sdk.NewDecFromStr(resp.TallyParams.Quorum)
		if err != nil {
			return nil, err
		}
		threshold, err := sdk.NewDecFromStr(resp.TallyParams.Threshold)
		if err != nil {
			return nil, err
		}
		vetoThreshold, err := sdk.NewDecFromStr(resp.TallyParams.VetoThreshold)
		if err != nil {
			return nil, err
		}

		response.TallyParams = v1beta1.NewTallyParams(quorum, threshold, vetoThreshold)
	}

	return response, nil
}

func (q legacyQueryServer) Deposit(c context.Context, req *v1beta1.QueryDepositRequest) (*v1beta1.QueryDepositResponse, error) {
	resp, err := q.keeper.Deposit(c, &v1.QueryDepositRequest{
		ProposalId: req.ProposalId,
		Depositor:  req.Depositor,
	})
	if err != nil {
		return nil, err
	}

	deposit := v3.ConvertToLegacyDeposit(resp.Deposit)
	return &v1beta1.QueryDepositResponse{Deposit: deposit}, nil
}

func (q legacyQueryServer) Deposits(c context.Context, req *v1beta1.QueryDepositsRequest) (*v1beta1.QueryDepositsResponse, error) {
	resp, err := q.keeper.Deposits(c, &v1.QueryDepositsRequest{
		ProposalId: req.ProposalId,
		Pagination: req.Pagination,
	})
	if err != nil {
		return nil, err
	}
	deposits := make([]v1beta1.Deposit, len(resp.Deposits))
	for idx, deposit := range resp.Deposits {
		deposits[idx] = v3.ConvertToLegacyDeposit(deposit)
	}

	return &v1beta1.QueryDepositsResponse{Deposits: deposits, Pagination: resp.Pagination}, nil
}

func (q legacyQueryServer) TallyResult(c context.Context, req *v1beta1.QueryTallyResultRequest) (*v1beta1.QueryTallyResultResponse, error) {
	resp, err := q.keeper.TallyResult(c, &v1.QueryTallyResultRequest{
		ProposalId: req.ProposalId,
	})
	if err != nil {
		return nil, err
	}

	tally, err := v3.ConvertToLegacyTallyResult(resp.Tally)
	if err != nil {
		return nil, err
	}

	return &v1beta1.QueryTallyResultResponse{Tally: tally}, nil
}
