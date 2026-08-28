using System;
using System.Collections.Generic;

namespace SubSteward.Plugin.M1
{
    /// <summary>
    /// Holds short-lived candidate bindings and fetched subtitle artifacts in memory.
    /// Raw provider IDs never leave this process.
    /// </summary>
    public sealed class M1TokenStore
    {
        private const int MaxCandidates = 200;
        private const int MaxArtifacts = 32;

        private readonly object sync = new object();
        private readonly Dictionary<string, M1CandidateRecord> candidates = new Dictionary<string, M1CandidateRecord>(StringComparer.Ordinal);
        private readonly Dictionary<string, M1ArtifactRecord> artifacts = new Dictionary<string, M1ArtifactRecord>(StringComparer.Ordinal);

        public string AddCandidate(Guid itemId, string mediaSourceId, string language, string rawId, bool titleMatch, bool hashMatch, string requestedLanguageVariant = null, string provider = null, string name = null, string format = null, bool likelyNonFullRelease = false)
        {
            var token = Guid.NewGuid().ToString("N");
            lock (sync)
            {
                PruneExpired(DateTime.UtcNow);
                EvictOldestCandidates();
                candidates[token] = new M1CandidateRecord
                {
                    Token = token,
                    ItemId = itemId,
                    MediaSourceId = mediaSourceId,
                    Language = language,
                    RequestedLanguageVariant = requestedLanguageVariant,
                    RawId = rawId,
                    TitleMatch = titleMatch,
                    HashMatch = hashMatch,
                    Provider = provider,
                    Name = name,
                    Format = format,
                    LikelyNonFullRelease = likelyNonFullRelease,
                    ExpiresAtUtc = DateTime.UtcNow.AddMinutes(10)
                };
            }

            return token;
        }

        public bool TryGetCandidate(string token, out M1CandidateRecord record)
        {
            lock (sync)
            {
                PruneExpired(DateTime.UtcNow);
                return candidates.TryGetValue(token ?? string.Empty, out record);
            }
        }

        public string AddArtifact(Guid itemId, string mediaSourceId, string language, byte[] content, M1ValidationResult validation, bool titleMatch, bool hashMatch, string requestedLanguageVariant = null, int timelineOffsetMilliseconds = 0)
        {
            var token = Guid.NewGuid().ToString("N");
            lock (sync)
            {
                PruneExpired(DateTime.UtcNow);
                EvictOldestArtifacts();
                artifacts[token] = new M1ArtifactRecord
                {
                    Token = token,
                    ItemId = itemId,
                    MediaSourceId = mediaSourceId,
                    Language = language,
                    RequestedLanguageVariant = requestedLanguageVariant,
                    Content = content,
                    Validation = validation,
                    TitleMatch = titleMatch,
                    HashMatch = hashMatch,
                    TimelineOffsetMilliseconds = timelineOffsetMilliseconds,
                    ExpiresAtUtc = DateTime.UtcNow.AddMinutes(20)
                };
            }

            return token;
        }

        public bool TryGetArtifact(string token, out M1ArtifactRecord record)
        {
            lock (sync)
            {
                PruneExpired(DateTime.UtcNow);
                return artifacts.TryGetValue(token ?? string.Empty, out record);
            }
        }

        public bool RemoveArtifact(string token)
        {
            lock (sync)
            {
                return artifacts.Remove(token ?? string.Empty);
            }
        }

        private void PruneExpired(DateTime nowUtc)
        {
            var expiredCandidates = new List<string>();
            foreach (var pair in candidates)
            {
                if (pair.Value.ExpiresAtUtc <= nowUtc)
                {
                    expiredCandidates.Add(pair.Key);
                }
            }

            foreach (var token in expiredCandidates)
            {
                candidates.Remove(token);
            }

            var expiredArtifacts = new List<string>();
            foreach (var pair in artifacts)
            {
                if (pair.Value.ExpiresAtUtc <= nowUtc)
                {
                    expiredArtifacts.Add(pair.Key);
                }
            }

            foreach (var token in expiredArtifacts)
            {
                artifacts.Remove(token);
            }
        }

        private void EvictOldestCandidates()
        {
            while (candidates.Count >= MaxCandidates)
            {
                string oldestToken = null;
                var oldestExpiry = DateTime.MaxValue;
                foreach (var pair in candidates)
                {
                    if (pair.Value.ExpiresAtUtc < oldestExpiry)
                    {
                        oldestExpiry = pair.Value.ExpiresAtUtc;
                        oldestToken = pair.Key;
                    }
                }

                if (oldestToken == null)
                {
                    break;
                }

                candidates.Remove(oldestToken);
            }
        }

        private void EvictOldestArtifacts()
        {
            while (artifacts.Count >= MaxArtifacts)
            {
                string oldestToken = null;
                var oldestExpiry = DateTime.MaxValue;
                foreach (var pair in artifacts)
                {
                    if (pair.Value.ExpiresAtUtc < oldestExpiry)
                    {
                        oldestExpiry = pair.Value.ExpiresAtUtc;
                        oldestToken = pair.Key;
                    }
                }

                if (oldestToken == null)
                {
                    break;
                }

                artifacts.Remove(oldestToken);
            }
        }
    }
}
