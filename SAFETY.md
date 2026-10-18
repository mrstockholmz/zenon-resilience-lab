# Safety limits (every session, human, local or cloud)

1. Devnet only. Generated disposable identities. Never use or reference real wallets, keyfiles, passphrases, or
   mainnet/testnet endpoints that are not started inside the session.
2. Never stress a shared network. No traffic leaves the VM except package/source downloads.
3. No secrets in files, logs, reports or the final message.
4. If a run reveals a crash or flaw reachable by a REMOTE party (peer, RPC client, wallet): do NOT write it into any
   file or public place. Put it ONLY in the final message, under a heading `PRIVATE-FINDING`, with minimized input,
   stack, reach. Local-only robustness issues may go into normal reports.
5. Respect the time budget. At the limit, stop and report what you have.
6. Report honestly: failed, skipped and unverified steps are stated as such. Simulated data is labelled simulated.
