using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using MediaBrowser.Controller.Api;
using MediaBrowser.Controller.Entities;
using MediaBrowser.Controller.Library;
using MediaBrowser.Controller.Net;
using MediaBrowser.Controller.Providers;
using MediaBrowser.Controller.Subtitles;
using MediaBrowser.Model.Configuration;
using MediaBrowser.Model.Dto;
using MediaBrowser.Model.IO;
using MediaBrowser.Model.Providers;
using MediaBrowser.Model.Services;
using SubSteward.Plugin.M2;

namespace SubSteward.Plugin.M1
{
    [Route("/SubSteward/Items", "GET", Summary = "Lists SubSteward-managed media items")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1ListItemsRequest
    {
        public string SearchTerm { get; set; }

        public int? Limit { get; set; }
    }

    [Route("/SubSteward/Items/{Id}", "GET", Summary = "Gets a SubSteward media item")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1GetItemRequest
    {
        public string Id { get; set; }
    }

    [Route("/SubSteward/Libraries", "GET", Summary = "Lists Emby media libraries for SubSteward settings")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1ListLibrariesRequest
    {
    }

    [Route("/SubSteward/Subtitles/Search", "GET", Summary = "Searches subtitle candidates")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1SearchSubtitlesRequest
    {
        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }
    }

    [Route("/SubSteward/Subtitles/Fetch", "POST", Summary = "Fetches and validates one subtitle candidate")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1FetchSubtitleRequest
    {
        public string CandidateToken { get; set; }
    }

    [Route("/SubSteward/Subtitles/Preview", "GET", Summary = "Previews one fetched subtitle artifact")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1PreviewSubtitleRequest
    {
        public string ArtifactToken { get; set; }
    }

    [Route("/SubSteward/Subtitles/Align", "POST", Summary = "Applies a manual uniform timeline offset to a fetched subtitle")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1AlignSubtitleRequest
    {
        public string ArtifactToken { get; set; }

        public int OffsetMilliseconds { get; set; }
    }

    [Route("/SubSteward/Subtitles/Install", "POST", Summary = "Installs one validated subtitle and refreshes the item")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1InstallSubtitleRequest
    {
        public string ArtifactToken { get; set; }
    }

    public sealed class M1ItemResponse
    {
        public string Id { get; set; }

        public string Name { get; set; }

        public string Type { get; set; }

        public bool IsStrm { get; set; }

        public string LibraryId { get; set; }

        public string LibraryName { get; set; }

        public List<M1SourceResponse> MediaSources { get; } = new List<M1SourceResponse>();

        public M2ActionReport Action { get; set; }
    }

    public sealed class M1LibraryResponse
    {
        public string Id { get; set; }

        public string Name { get; set; }
    }

    public sealed class M1SourceResponse
    {
        public string Id { get; set; }

        public string Name { get; set; }

        public string Container { get; set; }

        public bool IsRemote { get; set; }

        public int SubtitleStreamCount { get; set; }

        public string ExistingTargetHealth { get; set; }

        public List<M1SubtitleStreamResponse> SubtitleStreams { get; } = new List<M1SubtitleStreamResponse>();

        public M2PresenceReport Presence { get; set; }
    }

    public sealed class M1SubtitleStreamResponse
    {
        public bool IsExternal { get; set; }

        public string Language { get; set; }

        public string LanguageLabel { get; set; }

        public string Title { get; set; }

        public bool IsTargetLanguage { get; set; }

        public bool IsSecondaryLanguage { get; set; }

        public string Format { get; set; }

        public string Encoding { get; set; }

        public string Health { get; set; }

        public M2QualityReport Quality { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }

    public sealed class M1CandidateResponse
    {
        public string Token { get; set; }

        public string Provider { get; set; }

        public string Name { get; set; }

        public string Language { get; set; }

        public string LanguageLabel { get; set; }

        public string RequestedLanguageVariant { get; set; }

        public string Format { get; set; }

        public string Author { get; set; }

        public bool? IsHashMatch { get; set; }

        public bool TitleMatch { get; set; }

        public bool LanguageMismatch { get; set; }

        public bool VariantMismatch { get; set; }
    }

    public sealed class M1SearchResponse
    {
        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string LanguageLabel { get; set; }

        public string RequestedLanguageVariant { get; set; }

        public List<M1CandidateResponse> Candidates { get; } = new List<M1CandidateResponse>();
    }

    public sealed class M1ArtifactResponse
    {
        public string ArtifactToken { get; set; }

        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string LanguageLabel { get; set; }

        public string RequestedLanguageVariant { get; set; }

        public string Format { get; set; }

        public string Encoding { get; set; }

        public string Health { get; set; }

        public bool TitleMatch { get; set; }

        public bool HashMatch { get; set; }

        public int TimelineOffsetMilliseconds { get; set; }

        public List<string> Reasons { get; } = new List<string>();

        public M2QualityReport Quality { get; set; }

        public M2PreferenceReport Preference { get; set; }

        public M2ActionReport Action { get; set; }

        public List<M1Cue> Cues { get; } = new List<M1Cue>();
    }

    public sealed class M1InstallResponse
    {
        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string Format { get; set; }

        public string FileName { get; set; }

        public int ExternalSubtitleStreamCount { get; set; }
    }

    public sealed class M1SubtitleService : BaseApiService
    {
        private const int MaxCandidates = 20;
        private const int MaxPreviewCues = 200;
        private const int MaxExistingSubtitleInspections = 8;

        private static readonly M1TokenStore Store = new M1TokenStore();
        private static readonly M1SubtitleValidator Validator = new M1SubtitleValidator();
        private static readonly M1SubtitleFileInspector FileInspector = new M1SubtitleFileInspector(Validator);
        private static readonly M1SubtitleTimelineShifter TimelineShifter = new M1SubtitleTimelineShifter();
        private static readonly M2QualityAnalyzer QualityAnalyzer = new M2QualityAnalyzer();
        private static readonly M2PreferenceAnalyzer PreferenceAnalyzer = new M2PreferenceAnalyzer();
        private static readonly M2PresenceAnalyzer PresenceAnalyzer = new M2PresenceAnalyzer();
        private static readonly M2ActionAdvisor ActionAdvisor = new M2ActionAdvisor();

        private readonly ILibraryManager libraryManager;
        private readonly ISubtitleManager subtitleManager;
        private readonly IProviderManager providerManager;
        private readonly IFileSystem fileSystem;

        public M1SubtitleService(
            ILibraryManager libraryManager,
            ISubtitleManager subtitleManager,
            IProviderManager providerManager,
            IFileSystem fileSystem)
        {
            this.libraryManager = libraryManager;
            this.subtitleManager = subtitleManager;
            this.providerManager = providerManager;
            this.fileSystem = fileSystem;
        }

        public object Get(M1ListItemsRequest request)
        {
            var limit = request.Limit.GetValueOrDefault(50);
            if (limit < 1 || limit > 100)
            {
                throw new ArgumentException("Limit must be between 1 and 100.");
            }

            var items = libraryManager.GetItemList(new InternalItemsQuery
            {
                SearchTerm = request.SearchTerm ?? string.Empty,
                IncludeItemTypes = new[] { "Movie", "Episode" },
                Recursive = true,
                Limit = limit
            });

            return items.Select(item => ToItemResponse(item, false)).ToList();
        }

        public object Get(M1GetItemRequest request)
        {
            return ToItemResponse(ResolveItem(request.Id), true);
        }

        public object Get(M1ListLibrariesRequest request)
        {
            return libraryManager.GetVirtualFolders()
                .Where(folder => folder != null && !string.IsNullOrWhiteSpace(folder.ItemId))
                .Select(folder => new M1LibraryResponse
                {
                    Id = NormalizeLibraryId(folder.ItemId),
                    Name = folder.Name
                })
                .OrderBy(folder => folder.Name, StringComparer.OrdinalIgnoreCase)
                .ToList();
        }

        public async Task<object> Get(M1SearchSubtitlesRequest request)
        {
            var item = ResolveItem(request.ItemId);
            var source = ResolveSource(item, request.MediaSourceId);
            var requestedLanguage = M2Options.ParseTargetLanguage(request.Language);
            var language = requestedLanguage.Code;
            var candidates = (await subtitleManager.SearchSubtitles(item, language, false, false, false, Request.CancellationToken).ConfigureAwait(false) ?? Array.Empty<RemoteSubtitleInfo>())
                .OrderByDescending(candidate => candidate.IsHashMatch.GetValueOrDefault())
                .ThenByDescending(candidate => CandidateMatchesItem(candidate.Name, item))
                .ToList();
            var response = new M1SearchResponse
            {
                ItemId = item.Id.ToString("N"),
                MediaSourceId = source.Id,
                Language = language,
                LanguageLabel = requestedLanguage.Label,
                RequestedLanguageVariant = requestedLanguage.Variant
            };

            foreach (var candidate in candidates)
            {
                if (string.IsNullOrWhiteSpace(candidate.Id))
                {
                    continue;
                }

                var titleMatch = CandidateMatchesItem(candidate.Name, item);
                var hashMatch = candidate.IsHashMatch.GetValueOrDefault();
                var candidateLanguage = candidate.Language ?? language;
                var parsedCandidateLanguage = M2Language.Parse(candidateLanguage, language);
                var languageMismatch = !string.IsNullOrWhiteSpace(candidate.Language)
                    && !string.Equals(parsedCandidateLanguage.Code, language, StringComparison.OrdinalIgnoreCase);
                var variantMismatch = !string.IsNullOrWhiteSpace(candidate.Language)
                    && !string.IsNullOrWhiteSpace(parsedCandidateLanguage.Variant)
                    && !string.IsNullOrWhiteSpace(requestedLanguage.Variant)
                    && !string.Equals(parsedCandidateLanguage.Variant, requestedLanguage.Variant, StringComparison.OrdinalIgnoreCase);
                response.Candidates.Add(new M1CandidateResponse
                {
                    Token = Store.AddCandidate(item.Id, source.Id, language, candidate.Id, titleMatch, hashMatch, requestedLanguage.Variant),
                    Provider = candidate.ProviderName,
                    Name = candidate.Name,
                    Language = candidateLanguage,
                    LanguageLabel = parsedCandidateLanguage.Label,
                    RequestedLanguageVariant = requestedLanguage.Variant,
                    Format = candidate.Format,
                    Author = candidate.Author,
                    IsHashMatch = candidate.IsHashMatch,
                    TitleMatch = titleMatch,
                    LanguageMismatch = languageMismatch,
                    VariantMismatch = variantMismatch
                });

                if (response.Candidates.Count >= MaxCandidates)
                {
                    break;
                }
            }

            return response;
        }

        public async Task<object> Post(M1FetchSubtitleRequest request)
        {
            M1CandidateRecord candidate;
            if (!Store.TryGetCandidate(request.CandidateToken, out candidate))
            {
                throw new ArgumentException("Candidate token is expired or invalid.");
            }

            var item = ResolveItem(candidate.ItemId.ToString("N"));
            var source = ResolveSource(item, candidate.MediaSourceId);
            if (!candidate.TitleMatch && !candidate.HashMatch)
            {
                throw new InvalidOperationException("Candidate metadata does not match the selected Item.");
            }

            var fetched = await subtitleManager.GetRemoteSubtitles(candidate.RawId, Request.CancellationToken).ConfigureAwait(false);
            if (fetched == null || fetched.Stream == null)
            {
                throw new InvalidOperationException("Emby returned no subtitle content.");
            }

            byte[] content;
            using (fetched.Stream)
            {
                content = await ReadBoundedAsync(fetched.Stream, Request.CancellationToken).ConfigureAwait(false);
            }
            var validation = Validator.Validate(content, fetched.Format);
            if (!validation.IsUsable)
            {
                throw new InvalidOperationException("Fetched subtitle failed validation.");
            }

            if (IsChineseLanguage(candidate.Language) && !validation.HasHanCharacters)
            {
                throw new InvalidOperationException("Fetched subtitle metadata says Chinese, but its content contains no Chinese characters.");
            }

            var artifactToken = Store.AddArtifact(item.Id, source.Id, candidate.Language, content, validation, candidate.TitleMatch, candidate.HashMatch, candidate.RequestedLanguageVariant);
            return ToArtifactResponse(artifactToken, item.Id, source.Id, candidate.Language, validation, candidate.TitleMatch, candidate.HashMatch, candidate.RequestedLanguageVariant);
        }

        public object Get(M1PreviewSubtitleRequest request)
        {
            M1ArtifactRecord artifact;
            if (!Store.TryGetArtifact(request.ArtifactToken, out artifact))
            {
                throw new ArgumentException("Artifact token is expired or invalid.");
            }

            return ToArtifactResponse(
                artifact.Token,
                artifact.ItemId,
                artifact.MediaSourceId,
                artifact.Language,
                artifact.Validation,
                artifact.TitleMatch,
                artifact.HashMatch,
                artifact.RequestedLanguageVariant,
                artifact.TimelineOffsetMilliseconds);
        }

        public object Post(M1AlignSubtitleRequest request)
        {
            M1ArtifactRecord artifact;
            if (!Store.TryGetArtifact(request.ArtifactToken, out artifact))
            {
                throw new ArgumentException("Artifact token is expired or invalid.");
            }

            if (artifact.Validation.HasReplacementCharacter
                || string.Equals(artifact.Validation.Encoding, "unknown (replacement)", StringComparison.Ordinal))
            {
                throw new InvalidOperationException("The subtitle cannot be aligned because its encoding cannot be decoded losslessly; aligning would corrupt the content.");
            }

            var cumulativeOffset = (long)artifact.TimelineOffsetMilliseconds + request.OffsetMilliseconds;
            if (Math.Abs(cumulativeOffset) > M1SubtitleTimelineShifter.MaxAbsoluteOffsetMilliseconds)
            {
                throw new ArgumentOutOfRangeException(
                    nameof(request.OffsetMilliseconds),
                    "Cumulative timeline offset cannot exceed 10 minutes in either direction.");
            }

            var shiftedContent = TimelineShifter.Shift(artifact.Content, artifact.Validation.Format, request.OffsetMilliseconds);
            var shiftedValidation = Validator.Validate(shiftedContent, artifact.Validation.Format);
            if (!shiftedValidation.IsUsable)
            {
                throw new InvalidOperationException("Shifted subtitle failed validation.");
            }

            if (IsChineseLanguage(artifact.Language) && !shiftedValidation.HasHanCharacters)
            {
                throw new InvalidOperationException("Shifted subtitle no longer passes the target-language content gate.");
            }

            var item = ResolveItem(artifact.ItemId.ToString("N"));
            var source = ResolveSource(item, artifact.MediaSourceId);
            var token = Store.AddArtifact(
                item.Id,
                source.Id,
                artifact.Language,
                shiftedContent,
                shiftedValidation,
                artifact.TitleMatch,
                artifact.HashMatch,
                artifact.RequestedLanguageVariant,
                (int)cumulativeOffset);
            return ToArtifactResponse(
                token,
                item.Id,
                source.Id,
                artifact.Language,
                shiftedValidation,
                artifact.TitleMatch,
                artifact.HashMatch,
                artifact.RequestedLanguageVariant,
                (int)cumulativeOffset);
        }

        public async Task<object> Post(M1InstallSubtitleRequest request)
        {
            M1ArtifactRecord artifact;
            if (!Store.TryGetArtifact(request.ArtifactToken, out artifact))
            {
                throw new ArgumentException("Artifact token is expired or invalid.");
            }

            var item = ResolveItem(artifact.ItemId.ToString("N"));
            var source = ResolveSource(item, artifact.MediaSourceId);
            var before = CountExternalSubtitleStreams(item);
            string targetPath = null;
            try
            {
                targetPath = WriteSidecar(item, source, artifact);
                await providerManager.RefreshFullItem(item, new MetadataRefreshOptions(new DirectoryService(fileSystem)), Request.CancellationToken).ConfigureAwait(false);
                var refreshed = libraryManager.GetItemById(item.Id);
                var after = refreshed == null ? before : CountExternalSubtitleStreams(refreshed);
                if (after <= before || !HasExternalSubtitleStream(refreshed, source.Id, targetPath))
                {
                    throw new InvalidOperationException("Emby did not report the newly installed subtitle stream after refresh.");
                }

                Store.RemoveArtifact(artifact.Token);
                return new M1InstallResponse
                {
                    ItemId = item.Id.ToString("N"),
                    MediaSourceId = source.Id,
                    Language = artifact.Language,
                    Format = artifact.Validation.Format,
                    FileName = Path.GetFileName(targetPath),
                    ExternalSubtitleStreamCount = after
                };
            }
            catch
            {
                if (!string.IsNullOrWhiteSpace(targetPath))
                {
                    TryDeleteCreatedFile(targetPath);
                }

                throw;
            }
        }

        private BaseItem ResolveItem(string itemId)
        {
            Guid parsed;
            if (!Guid.TryParse(itemId, out parsed))
            {
                throw new ArgumentException("ItemId is invalid.");
            }

            var item = libraryManager.GetItemById(parsed);
            if (item == null || (!string.Equals(item.GetType().Name, "Movie", StringComparison.Ordinal) && !string.Equals(item.GetType().Name, "Episode", StringComparison.Ordinal)))
            {
                throw new ArgumentException("Item is not a supported Movie or Episode.");
            }

            return item;
        }

        private static MediaSourceInfo ResolveSource(BaseItem item, string mediaSourceId)
        {
            var sources = item.GetMediaSources(false, false, new LibraryOptions()).ToList();
            if (sources.Count != 1)
            {
                throw new InvalidOperationException("M1 currently requires exactly one MediaSource.");
            }

            var source = sources[0];
            if (!string.IsNullOrWhiteSpace(mediaSourceId) && !string.Equals(source.Id, mediaSourceId, StringComparison.Ordinal))
            {
                throw new ArgumentException("MediaSourceId does not belong to the selected Item.");
            }

            return source;
        }

        private static string NormalizeLanguage(string language)
        {
            return M2Options.ParseTargetLanguage(language).Code;
        }

        private static string NormalizeLibraryId(string libraryId)
        {
            Guid parsed;
            return Guid.TryParse(libraryId, out parsed) ? parsed.ToString("N") : libraryId;
        }

        private static string GetTargetLanguage(LibraryPreferenceOverride libraryOverride = null)
        {
            var configuration = Plugin.Instance?.Configuration;
            return M2Options.CanonicalizeTargetLanguage(
                libraryOverride != null && libraryOverride.Enabled && !string.IsNullOrWhiteSpace(libraryOverride.TargetLanguage)
                    ? libraryOverride.TargetLanguage
                    : configuration?.TargetLanguage);
        }

        private static string GetSecondaryLanguage(LibraryPreferenceOverride libraryOverride = null)
        {
            var configuration = Plugin.Instance?.Configuration;
            return M2Options.CanonicalizeSecondaryLanguage(
                libraryOverride != null && libraryOverride.Enabled && !string.IsNullOrWhiteSpace(libraryOverride.SecondaryLanguage)
                    ? libraryOverride.SecondaryLanguage
                    : configuration?.SecondaryLanguage);
        }

        private static M2PreferenceOptions GetPreferenceOptions(LibraryPreferenceOverride libraryOverride = null)
        {
            var configuration = Plugin.Instance?.Configuration;
            return M2Options.ParsePreferenceOptions(
                libraryOverride != null && libraryOverride.Enabled
                    ? libraryOverride.PreferBilingual
                    : configuration?.PreferBilingual ?? false,
                libraryOverride != null && libraryOverride.Enabled && !string.IsNullOrWhiteSpace(libraryOverride.FormatOrder)
                    ? libraryOverride.FormatOrder
                    : configuration?.FormatOrder);
        }

        private LibraryPreferenceOverride GetLibraryOverride(BaseItem item)
        {
            var folders = libraryManager.GetCollectionFolders(item);
            var folder = folders == null ? null : folders.FirstOrDefault();
            var configuration = Plugin.Instance?.Configuration;
            if (folder == null || configuration?.LibraryOverrides == null)
            {
                return null;
            }

            var libraryId = folder.Id.ToString("N");
            return configuration.LibraryOverrides.FirstOrDefault(entry =>
                entry != null && string.Equals(NormalizeLibraryId(entry.LibraryId), libraryId, StringComparison.OrdinalIgnoreCase));
        }

        private static bool IsChineseLanguage(string language)
        {
            return M2Language.IsChinese(language);
        }

        private static bool CandidateMatchesItem(string candidateName, BaseItem item)
        {
            if (string.IsNullOrWhiteSpace(candidateName))
            {
                return false;
            }

            return (!string.IsNullOrWhiteSpace(item.Name) && candidateName.IndexOf(item.Name, StringComparison.OrdinalIgnoreCase) >= 0)
                || (!string.IsNullOrWhiteSpace(item.OriginalTitle) && candidateName.IndexOf(item.OriginalTitle, StringComparison.OrdinalIgnoreCase) >= 0);
        }

        private M1ItemResponse ToItemResponse(BaseItem item, bool inspectExistingSubtitles)
        {
            var folders = libraryManager.GetCollectionFolders(item);
            var folder = folders == null ? null : folders.FirstOrDefault();
            var libraryOverride = GetLibraryOverride(item);
            var targetLanguage = GetTargetLanguage(libraryOverride);
            var secondaryLanguage = GetSecondaryLanguage(libraryOverride);
            var response = new M1ItemResponse
            {
                Id = item.Id.ToString("N"),
                Name = item.Name,
                Type = item.GetType().Name,
                IsStrm = !string.IsNullOrWhiteSpace(item.Path) && item.Path.EndsWith(".strm", StringComparison.OrdinalIgnoreCase),
                LibraryId = folder?.Id.ToString("N"),
                LibraryName = folder?.Name
            };

            foreach (var source in item.GetMediaSources(false, false, new LibraryOptions()))
            {
                var subtitleStreams = source.MediaStreams == null
                    ? new List<M2SubtitleStreamSnapshot>()
                    : source.MediaStreams
                        .Where(stream => stream.Type.ToString() == "Subtitle")
                        .Select(stream => new M2SubtitleStreamSnapshot
                        {
                            IsExternal = stream.IsExternal,
                            Language = stream.Language,
                            Title = string.IsNullOrWhiteSpace(stream.DisplayTitle) ? stream.Title : stream.DisplayTitle,
                            Path = stream.IsExternal ? stream.Path : null
                        })
                        .ToList();

                var sourceResponse = new M1SourceResponse
                {
                    Id = source.Id,
                    Name = source.Name,
                    Container = source.Container,
                    IsRemote = source.IsRemote,
                    SubtitleStreamCount = source.MediaStreams == null ? 0 : source.MediaStreams.Count(stream => stream.Type.ToString() == "Subtitle"),
                    Presence = PresenceAnalyzer.Analyze(
                        subtitleStreams,
                        targetLanguage,
                        secondaryLanguage)
                };

                if (inspectExistingSubtitles)
                {
                    PopulateExistingSubtitleStreams(
                        item,
                        source,
                        sourceResponse,
                        targetLanguage,
                        secondaryLanguage);
                    sourceResponse.ExistingTargetHealth = DetermineExistingTargetHealth(sourceResponse);
                }

                response.MediaSources.Add(sourceResponse);
            }

            var existingTargetHealth = inspectExistingSubtitles && response.MediaSources.Count == 1
                ? response.MediaSources[0].ExistingTargetHealth
                : null;
            response.Action = ActionAdvisor.Advise(new M2ActionInput
            {
                SourceCount = response.MediaSources.Count,
                TargetLanguagePresent = response.MediaSources.Count == 1 && response.MediaSources[0].Presence != null
                    ? (bool?)response.MediaSources[0].Presence.TargetLanguagePresent
                    : null,
                ExistingTargetHealth = existingTargetHealth
            });

            return response;
        }

        private void PopulateExistingSubtitleStreams(
            BaseItem item,
            MediaSourceInfo source,
            M1SourceResponse sourceResponse,
            string targetLanguage,
            string secondaryLanguage)
        {
            var streams = (source.MediaStreams ?? new List<MediaBrowser.Model.Entities.MediaStream>())
                .Where(stream => stream.Type.ToString() == "Subtitle")
                .Select(stream => new
                {
                    Stream = stream,
                    Snapshot = new M2SubtitleStreamSnapshot
                    {
                        IsExternal = stream.IsExternal,
                        Language = stream.Language,
                        Title = string.IsNullOrWhiteSpace(stream.DisplayTitle) ? stream.Title : stream.DisplayTitle,
                        Path = stream.IsExternal ? stream.Path : null
                    }
                })
                .OrderByDescending(entry => PresenceAnalyzer.MatchesConfiguredLanguage(entry.Snapshot, targetLanguage))
                .ThenByDescending(entry => PresenceAnalyzer.MatchesConfiguredLanguage(entry.Snapshot, secondaryLanguage))
                .ThenByDescending(entry => entry.Stream.IsExternal)
                .ToList();

            var inspectionCount = 0;
            foreach (var entry in streams)
            {
                var isTargetLanguage = PresenceAnalyzer.MatchesConfiguredLanguage(entry.Snapshot, targetLanguage);
                var isSecondaryLanguage = PresenceAnalyzer.MatchesConfiguredLanguage(entry.Snapshot, secondaryLanguage);
                var parsedLanguage = string.IsNullOrWhiteSpace(entry.Stream.Language)
                    ? null
                    : M2Language.Parse(entry.Stream.Language, entry.Stream.Language);
                var detail = new M1SubtitleStreamResponse
                {
                    IsExternal = entry.Stream.IsExternal,
                    Language = entry.Stream.Language,
                    LanguageLabel = parsedLanguage == null
                        ? (isTargetLanguage ? M2Language.Parse(targetLanguage, targetLanguage).Label : "未知语言")
                        : parsedLanguage.Label,
                    Title = string.IsNullOrWhiteSpace(entry.Stream.DisplayTitle) ? entry.Stream.Title : entry.Stream.DisplayTitle,
                    IsTargetLanguage = isTargetLanguage,
                    IsSecondaryLanguage = isSecondaryLanguage,
                    Health = "UNKNOWN"
                };

                if (!entry.Stream.IsExternal)
                {
                    detail.Reasons.Add("内封字幕默认不提取正文，当前无法深检。即使存在目标语言，也不会自动修改。");
                }
                else if (inspectionCount >= MaxExistingSubtitleInspections)
                {
                    detail.Reasons.Add("当前条目已达到外置字幕深检数量上限。");
                }
                else
                {
                    inspectionCount++;
                    var inspection = FileInspector.Inspect(
                        GetInspectionAnchor(item, source),
                        entry.Stream.Path,
                        GetSubtitleFormat(entry.Stream.Path));
                    if (!inspection.IsInspectable || inspection.Validation == null)
                    {
                        detail.Reasons.AddRange(inspection.Reasons);
                    }
                    else
                    {
                        detail.Format = inspection.Validation.Format;
                        detail.Encoding = inspection.Validation.Encoding;
                        detail.Health = inspection.Validation.Health;
                        detail.Reasons.AddRange(inspection.Reasons);
                        detail.Quality = QualityAnalyzer.Analyze(inspection.Validation, targetLanguage, secondaryLanguage);
                    }
                }

                sourceResponse.SubtitleStreams.Add(detail);
            }
        }

        private static string DetermineExistingTargetHealth(M1SourceResponse source)
        {
            var targetStreams = source.SubtitleStreams
                .Where(stream => stream.IsTargetLanguage)
                .ToList();
            if (targetStreams.Count == 0)
            {
                return null;
            }

            if (targetStreams.Any(stream => string.Equals(stream.Health, "PASS", StringComparison.OrdinalIgnoreCase)))
            {
                return "PASS";
            }

            if (targetStreams.Any(stream => string.Equals(stream.Health, "WARNING", StringComparison.OrdinalIgnoreCase)))
            {
                return "WARNING";
            }

            if (targetStreams.All(stream => string.Equals(stream.Health, "FAIL", StringComparison.OrdinalIgnoreCase)))
            {
                return "FAIL";
            }

            return null;
        }

        private static string GetInspectionAnchor(BaseItem item, MediaSourceInfo source)
        {
            if (!string.IsNullOrWhiteSpace(item.Path) && item.Path.EndsWith(".strm", StringComparison.OrdinalIgnoreCase))
            {
                return item.Path;
            }

            return source.IsRemote ? null : source.Path;
        }

        private static string GetSubtitleFormat(string path)
        {
            if (string.IsNullOrWhiteSpace(path))
            {
                return null;
            }

            try
            {
                return Path.GetExtension(path);
            }
            catch (ArgumentException)
            {
                return null;
            }
        }

        private M1ArtifactResponse ToArtifactResponse(string token, Guid itemId, string sourceId, string language, M1ValidationResult validation, bool titleMatch, bool hashMatch, string requestedLanguageVariant, int timelineOffsetMilliseconds = 0)
        {
            var response = new M1ArtifactResponse
            {
                ArtifactToken = token,
                ItemId = itemId.ToString("N"),
                MediaSourceId = sourceId,
                Language = language,
                LanguageLabel = M2Options.ParseTargetLanguage(language).Label,
                RequestedLanguageVariant = requestedLanguageVariant,
                Format = validation.Format,
                Encoding = validation.Encoding,
                Health = validation.Health,
                TitleMatch = titleMatch,
                HashMatch = hashMatch,
                TimelineOffsetMilliseconds = timelineOffsetMilliseconds
            };
            response.Reasons.AddRange(validation.Reasons);
            var item = libraryManager.GetItemById(itemId);
            var libraryOverride = item == null ? null : GetLibraryOverride(item);
            var secondaryLanguage = GetSecondaryLanguage(libraryOverride);
            var preferenceOptions = GetPreferenceOptions(libraryOverride);
            response.Quality = QualityAnalyzer.Analyze(validation, language, secondaryLanguage);
            response.Preference = PreferenceAnalyzer.Evaluate(
                validation,
                language,
                secondaryLanguage,
                token,
                titleMatch,
                hashMatch,
                preferenceOptions);
            response.Action = ActionAdvisor.Advise(new M2ActionInput
            {
                SourceCount = 1,
                TargetLanguagePresent = null,
                CandidateAvailable = true,
                CandidateHealth = response.Quality.Health,
                PreferenceSuitability = response.Preference.Suitability,
                TitleMatch = titleMatch,
                HashMatch = hashMatch,
                PreferBilingual = preferenceOptions.PreferBilingual,
                BilingualDetected = response.Quality.BilingualDetected,
                BilingualConfidence = response.Quality.BilingualConfidence
            });
            response.Cues.AddRange(validation.Cues.Take(MaxPreviewCues));
            return response;
        }

        private static int CountExternalSubtitleStreams(BaseItem item)
        {
            return item.GetMediaSources(false, false, new LibraryOptions())
                .Sum(source => source.MediaStreams == null ? 0 : source.MediaStreams.Count(stream => stream.IsExternal && stream.Type.ToString() == "Subtitle"));
        }

        private static async Task<byte[]> ReadBoundedAsync(Stream stream, CancellationToken cancellationToken)
        {
            const int maxBytes = 16 * 1024 * 1024;
            using (var output = new MemoryStream())
            {
                var buffer = new byte[81920];
                while (true)
                {
                    var read = await stream.ReadAsync(buffer, 0, buffer.Length, cancellationToken).ConfigureAwait(false);
                    if (read == 0)
                    {
                        break;
                    }

                    if (output.Length + read > maxBytes)
                    {
                        throw new InvalidOperationException("Fetched subtitle exceeds the M1 size limit.");
                    }

                    output.Write(buffer, 0, read);
                }

                return output.ToArray();
            }
        }

        private static string WriteSidecar(BaseItem item, MediaSourceInfo source, M1ArtifactRecord artifact)
        {
            var anchor = item.Path;
            if (!string.IsNullOrWhiteSpace(item.Path) && item.Path.EndsWith(".strm", StringComparison.OrdinalIgnoreCase))
            {
                EnsureRegularFile(anchor, "STRM Item.Path");
            }
            else
            {
                if (source.IsRemote || string.IsNullOrWhiteSpace(source.Path))
                {
                    throw new InvalidOperationException("The selected non-STRM source is not a local filesystem path.");
                }

                anchor = source.Path;
                EnsureRegularFile(anchor, "MediaSource.Path");
            }

            var directory = Path.GetDirectoryName(Path.GetFullPath(anchor));
            var baseName = Path.GetFileNameWithoutExtension(anchor);
            if (string.IsNullOrWhiteSpace(directory) || string.IsNullOrWhiteSpace(baseName))
            {
                throw new InvalidOperationException("The selected media path cannot produce a sidecar target.");
            }

            var directoryAttributes = File.GetAttributes(directory);
            if ((directoryAttributes & FileAttributes.Directory) == 0 || (directoryAttributes & FileAttributes.ReparsePoint) != 0)
            {
                throw new InvalidOperationException("The selected media directory is not a regular directory.");
            }

            var extension = artifact.Validation.Format == "ass" || artifact.Validation.Format == "ssa" ? artifact.Validation.Format : "srt";
            var requestedVariant = M2Language.Parse(artifact.RequestedLanguageVariant, artifact.Language).Variant;
            var languageTag = M2Options.ResolveSubtitleLanguageTag(requestedVariant, artifact.Language);
            var contentTypeLabel = M2SidecarLabel.Build(artifact.Validation, requestedVariant);

            var stem = baseName;
            if (!string.IsNullOrWhiteSpace(contentTypeLabel))
            {
                stem += "." + contentTypeLabel;
            }

            stem += "." + languageTag;
            for (var version = 0; version < 100; version++)
            {
                var suffix = version == 0 ? string.Empty : ".v" + version;
                var target = Path.Combine(directory, stem + suffix + "." + extension);
                if (File.Exists(target))
                {
                    continue;
                }

                var temporary = Path.Combine(directory, "." + Path.GetFileName(target) + ".substeward-" + Guid.NewGuid().ToString("N") + ".tmp");
                try
                {
                    using (var output = new FileStream(temporary, FileMode.CreateNew, FileAccess.Write, FileShare.None))
                    {
                        output.Write(artifact.Content, 0, artifact.Content.Length);
                        output.Flush(true);
                    }

                    File.Move(temporary, target);
                    return target;
                }
                finally
                {
                    TryDeleteCreatedFile(temporary);
                }
            }

            throw new IOException("Unable to allocate a new subtitle sidecar filename.");
        }

        private static void EnsureRegularFile(string path, string label)
        {
            if (string.IsNullOrWhiteSpace(path) || !File.Exists(path))
            {
                throw new InvalidOperationException(label + " does not point to an existing file.");
            }

            var attributes = File.GetAttributes(path);
            if ((attributes & FileAttributes.Directory) != 0 || (attributes & FileAttributes.ReparsePoint) != 0)
            {
                throw new InvalidOperationException(label + " is not a regular file.");
            }
        }

        private static bool HasExternalSubtitleStream(BaseItem item, string sourceId, string targetPath)
        {
            string fullTargetPath;
            if (!TryGetFullPath(targetPath, out fullTargetPath))
            {
                return false;
            }

            return item.GetMediaSources(false, false, new LibraryOptions())
                .Where(source => string.Equals(source.Id, sourceId, StringComparison.Ordinal))
                .SelectMany(source => source.MediaStreams ?? new List<MediaBrowser.Model.Entities.MediaStream>())
                .Any(stream => stream.IsExternal
                    && stream.Type.ToString() == "Subtitle"
                    && PathsEqual(stream.Path, fullTargetPath));
        }

        private static bool PathsEqual(string path, string expectedFullPath)
        {
            string fullPath;
            return TryGetFullPath(path, out fullPath)
                && string.Equals(fullPath, expectedFullPath, StringComparison.OrdinalIgnoreCase);
        }

        private static bool TryGetFullPath(string path, out string fullPath)
        {
            fullPath = null;
            if (string.IsNullOrWhiteSpace(path))
            {
                return false;
            }

            try
            {
                fullPath = Path.GetFullPath(path);
                return true;
            }
            catch (ArgumentException)
            {
                return false;
            }
            catch (NotSupportedException)
            {
                return false;
            }
        }

        private static void TryDeleteCreatedFile(string path)
        {
            try
            {
                if (!string.IsNullOrWhiteSpace(path) && File.Exists(path))
                {
                    File.Delete(path);
                }
            }
            catch (IOException)
            {
            }
            catch (UnauthorizedAccessException)
            {
            }
        }
    }
}
