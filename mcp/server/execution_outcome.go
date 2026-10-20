package server

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
)

func executionFailure(err error, progress *rpcProgress) (string, *SafeExecutionError) {
	unknown := func(code string) (string, *SafeExecutionError) {
		return ExecutionUnknown, &SafeExecutionError{Code: code, Message: "The operation's outcome could not be established. Partial effects are possible; do not retry automatically."}
	}
	if errors.Is(err, context.Canceled) {
		return unknown(CodeCancelled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return unknown(CodeTimeout)
	}
	var ne net.Error
	if errors.Is(err, ErrDaemonUnreachable) || errors.As(err, &ne) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return unknown(CodeDaemonUnreachable)
	}
	if progress.partialOrAmbiguous() {
		return unknown(CodeRPCError)
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		return ExecutionFailed, &SafeExecutionError{Code: rpcCode(rpcErr), Message: "The daemon reported an unsuccessful operation after approval. This does not establish that no partial effects occurred."}
	}
	var te *ToolError
	if errors.As(err, &te) {
		switch te.Code {
		case CodeInvalidArgument, CodePermissionDenied, CodeNotFound, CodeToolUnavailable,
			CodeFailedPrecondition, CodeSizeLimitExceeded, codeAlreadyExists, CodeEgressNotAcknowledged:
			return ExecutionFailed, &SafeExecutionError{Code: te.Code, Message: "The operation reported a definite failure after approval. This does not establish that no partial effects occurred."}
		case CodeCancelled, CodeTimeout, CodeDaemonUnreachable, CodeIndexUnavailable:
			return unknown(te.Code)
		}
	}
	return unknown(CodeInternal)
}

// successfulOutcome uses only known typed handler results. No output text is
// searched for CIDs, and no result reference is guessed from a tool argument.
// Partial stream/import reports do not acquire a successful receipt.
func successfulOutcome(out any) (state, effect string, refs []ReceiptReference, publicError *SafeExecutionError) {
	refs = make([]ReceiptReference, 0, 16)
	add := func(kind, value string) {
		if value != "" && len(value) <= 4096 && len(refs) < 16 {
			refs = append(refs, ReceiptReference{Kind: kind, Value: value})
		}
	}
	addCID := func(value string) {
		if _, err := cid.Decode(value); err == nil {
			add("cid", value)
		}
	}
	addKey := func(value string) {
		if _, err := peer.Decode(value); err == nil {
			add("key", value)
		}
	}
	addJob := func(value string) {
		if _, err := uuid.Parse(value); err == nil {
			add("job", value)
		}
	}
	partial := func() (string, string, []ReceiptReference, *SafeExecutionError) {
		return ExecutionUnknown, "", nil, &SafeExecutionError{Code: CodeRPCError, Message: "The daemon reported an incomplete operation. Some effects may have occurred; do not retry automatically."}
	}
	effect = "operation_completed"
	switch v := out.(type) {
	case ipfsAddOutput:
		addCID(v.CID)
	case dagPutOutput:
		addCID(v.CID)
	case carImportOutput:
		for _, root := range v.Roots {
			if root.PinError != "" {
				return partial()
			}
			addCID(root.CID)
		}
	case pinAddOutput:
		for _, value := range v.Pins {
			addCID(value)
		}
	case pinRmOutput:
		for _, value := range v.Unpinned {
			addCID(value)
		}
	case pinUpdateOutput:
		addCID(v.From)
		addCID(v.To)
	case mfsWriteOutput:
		addCID(v.CID)
	case mfsMkdirOutput:
		addCID(v.CID)
	case mfsCopyOutput:
		addCID(v.CID)
	case ipnsPublishOutput:
		// Name comes from name/publish, not the input key/path.
		if _, err := peer.Decode(v.Name); err == nil {
			add("ipns_name", v.Name)
		}
	case keyGenOutput:
		addKey(v.ID)
	case keyRenameOutput:
		addKey(v.ID)
	case keyRmOutput:
		for _, key := range v.Removed {
			addKey(key.ID)
		}
	case repoGcOutput:
		if len(v.Errors) > 0 {
			return partial()
		}
	case bulkStartOutput:
		// The accepted run may still be working or fail later.
		effect = "job_accepted"
		addJob(v.RunID)
	case bulkCancelOutput:
		addJob(v.RunID)
	}
	return ExecutionSucceeded, effect, refs, nil
}
