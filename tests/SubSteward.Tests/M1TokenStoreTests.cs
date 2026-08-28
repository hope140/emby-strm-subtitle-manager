using System;
using System.Collections.Generic;
using System.Linq;
using SubSteward.Plugin.M1;
using Xunit;

namespace SubSteward.Tests;

public sealed class M1TokenStoreTests
{
    [Fact]
    public void CandidateAndArtifactTokensResolveWithoutExposingRawProviderId()
    {
        var store = new M1TokenStore();
        var itemId = Guid.NewGuid();
        var candidateToken = store.AddCandidate(itemId, "source", "zho", "provider-private-id", true, false, "zh-Hans");

        Assert.NotEqual("provider-private-id", candidateToken);
        Assert.True(store.TryGetCandidate(candidateToken, out var candidate));
        Assert.Equal("provider-private-id", candidate.RawId);
        Assert.Equal("zh-Hans", candidate.RequestedLanguageVariant);

        var artifactToken = store.AddArtifact(itemId, "source", "zho", new byte[] { 1, 2 }, new M1ValidationResult { Format = "srt", Health = "PASS" }, true, false, candidate.RequestedLanguageVariant, 500);
        Assert.True(store.TryGetArtifact(artifactToken, out var artifact));
        Assert.Equal(itemId, artifact.ItemId);
        Assert.Equal("zh-Hans", artifact.RequestedLanguageVariant);
        Assert.Equal(500, artifact.TimelineOffsetMilliseconds);
        Assert.True(store.RemoveArtifact(artifactToken));
        Assert.False(store.TryGetArtifact(artifactToken, out _));
    }

    [Fact]
    public void ArtifactStore_EvictsOldestWhenOverCapacity()
    {
        var store = new M1TokenStore();
        var itemId = Guid.NewGuid();
        var tokens = new List<string>();
        for (var i = 0; i < 33; i++)
        {
            tokens.Add(store.AddArtifact(itemId, "source", "zho", new byte[] { (byte)i }, new M1ValidationResult { Format = "srt", Health = "PASS" }, true, false));
        }

        Assert.False(store.TryGetArtifact(tokens[0], out _));
        Assert.True(store.TryGetArtifact(tokens[^1], out _));
        Assert.Equal(32, tokens.Count(token => store.TryGetArtifact(token, out _)));
    }

    [Fact]
    public void CandidateStore_EvictsOldestWhenOverCapacity()
    {
        var store = new M1TokenStore();
        var itemId = Guid.NewGuid();
        var tokens = new List<string>();
        for (var i = 0; i < 201; i++)
        {
            tokens.Add(store.AddCandidate(itemId, "source", "zho", "provider-" + i, true, false));
        }

        Assert.False(store.TryGetCandidate(tokens[0], out _));
        Assert.True(store.TryGetCandidate(tokens[^1], out _));
        Assert.Equal(200, tokens.Count(token => store.TryGetCandidate(token, out _)));
    }
}
