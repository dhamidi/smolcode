# The setup

After reviewing `agent.go`, here's an explanation of the current caching implementation and how it relates to sending only uncached messages:

**Current Caching Mechanism (`runInference` function):**

1. **Turn-Specific Cache Creation:**
    * At the beginning of each `runInference` call (which represents a turn in the conversation), the agent attempts to create a *new* cache. This cache is named `smolcode-turncache-<agent_name>-<timestamp>` and has a short TTL (5 minutes).
    * This "turn cache" includes:
        * The system instruction (if any).
        * The list of available tools.
        * The **entire current conversation history** (`cacheConfig.Contents = conversation`).
    * If this turn-specific cache is created successfully (`turnCacheName != ""`), its name is stored in `turnCacheName`.
    * A `defer` function is set up to delete this turn-specific cache when `runInference` exits. This ensures these short-lived caches are cleaned up.

2. **Using the Cache in `GenerateContent`:**
    * When calling `agent.client.Models.GenerateContent`, if `turnCacheName` is not empty (meaning the turn-specific cache was successfully created):
        * `config.CachedContent` is set to `turnCacheName`.
        * Crucially, the `SystemInstruction`, `Tools`, and the `conversation` argument to `GenerateContent` are **NOT** set again in the request. The API documentation implies that if `CachedContent` is provided, the other fields (like the `conversation` argument itself) are ignored by the backend as they are already part of the specified cache.
    * If `turnCacheName` is empty (e.g., turn-specific cache creation failed):
        * `config.Tools` and `config.SystemInstruction` are set directly in the `GenerateContentConfig`.
        * The `conversation` argument passed to `GenerateContent` (which is `agent.history`) is used directly by the API.

**Addressing "Only Send Uncached Messages":**

The current implementation **does not strictly send only *new* (uncached) messages** in the way one might interpret "sending uncached messages as part of the conversation."

Instead, it does the following:

* **For each turn, it creates a cache of the *entire history up to that point*, plus system instructions and tools.**
* It then tells the `GenerateContent` API to use this newly created cache.

**What this means:**

* **The entire context (history, tools, system prompt) is uploaded to create a cache for *every single turn*.**
* Then, the `GenerateContent` call refers to this just-created cache.
* The benefit here is that the `GenerateContent` call itself is very small (just a reference to the cache name). The actual content (history, etc.) is uploaded during the `Caches.Create` call.

**How it differs from a more traditional "send only new messages" approach:**

A more traditional interpretation might involve:

1. Having a more persistent base cache that includes, say, `agent.history` up to `N-1` messages.
2. For the current turn, sending only the new user message (and perhaps recent tool calls/responses) along with a reference to that base cache.

The current code *does* have a concept of a longer-lived cache (`agent.cachedContent` and `agent.refreshCache`). However, `refreshCache` aims to create a cache of the *entire current history* whenever the history grows. The `runInference` function then *re-caches this entire history again* for the specific turn into `turnCacheName`.

**In summary:**

* The code *uses* caching for every API call by creating a short-lived, turn-specific cache that contains the *entire current conversation context*.
* The `GenerateContent` call itself then becomes lightweight as it just points to this cache.
* However, it's not "sending only uncached messages" in the sense of incrementally adding new messages to an existing cache reference for the `GenerateContent` call. Instead, it effectively re-uploads the whole context into a new cache for each turn.

The `agent.refreshCache()` method seems like an attempt at a more persistent cache, but the `runInference` logic currently creates a new, comprehensive "turn cache" regardless of what `agent.cachedContent` might hold. The `agent.cachedContent` (the longer-lived one) is not directly used by `runInference` for the `GenerateContent` call. It seems `refreshCache` is called in the main `Run` loop, potentially creating/updating `agent.cachedContent`, but then `runInference` immediately creates its own `turnCacheName`.

This might be an intentional design to ensure the `GenerateContent` call is always as minimal as possible, relying on the `Caches.Create` to handle the bulk of the data transfer for that turn.

# What you need to do

I think we send the conversation twice: as cached content and as the full conversation slice in the call to GenerateContent.  We need to change it so that only messages which aren't part of the cache are sent from the conversation slice.  Please make the necessary changes.
