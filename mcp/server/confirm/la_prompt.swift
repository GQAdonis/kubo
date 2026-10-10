// LocalAuthentication helper for the IPFS MCP server. It is interpreted by
// /usr/bin/swift from stdin and is never written to disk.
//
// Environment:
//   IPFS_MCP_LA_PROBE=1    only check that the policy can be evaluated
//   IPFS_MCP_LA_REASON     the text the system dialog shows
//
// Exit codes: 0 authenticated, 2 unavailable, 3 not authenticated.
import Foundation
import LocalAuthentication

let env = ProcessInfo.processInfo.environment
let ctx = LAContext()
var policyError: NSError?
guard ctx.canEvaluatePolicy(.deviceOwnerAuthentication, error: &policyError) else {
    FileHandle.standardError.write(Data("LocalAuthentication unavailable: \(policyError?.localizedDescription ?? "unknown")\n".utf8))
    exit(2)
}
if env["IPFS_MCP_LA_PROBE"] == "1" {
    exit(0)
}
let reason = env["IPFS_MCP_LA_REASON"] ?? "Approve an IPFS MCP operation"
let done = DispatchSemaphore(value: 0)
var authenticated = false
ctx.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason) { ok, error in
    authenticated = ok
    if !ok {
        FileHandle.standardError.write(Data("not authenticated: \(error?.localizedDescription ?? "unknown")\n".utf8))
    }
    done.signal()
}
done.wait()
exit(authenticated ? 0 : 3)
