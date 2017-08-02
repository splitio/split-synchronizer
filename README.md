# Split Synchronizer
 > **split-sync** A background service to synchronize Split information with your SDK

Split synchronizer is able to run in 2 different modes.
 - **Producer mode** (default): coordinates the sending and receiving of data to a **remote datastore** that all of your processes can share to pull data for the evaluation of treatments.
 - **Proxy mode**: keep synchronized SDKs connecting they with split-sync proxy to reduce connection latencies and letting the proxy receive information and post impressions to Split servers.
