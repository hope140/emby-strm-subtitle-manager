using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using MediaBrowser.Controller.Entities;
using MediaBrowser.Controller.Entities.TV;
using MediaBrowser.Controller.Library;
using MediaBrowser.Controller.Providers;
using MediaBrowser.Controller.Subtitles;
using MediaBrowser.Model.Configuration;
using MediaBrowser.Model.Dto;
using MediaBrowser.Model.IO;
using MediaBrowser.Model.Logging;
using MediaBrowser.Model.Providers;
using MediaBrowser.Model.Entities;
using SubSteward.Plugin.M1;
using SubSteward.Plugin.M2;

namespace SubSteward.Plugin.M3
{
    /// <summary>
    /// Executes one bounded M3 supplementation pass. Provider IDs and
    /// subtitle bytes remain local to a single item attempt and are never
    /// returned in the run result or written to logs.
    /// </summary>
    public sealed class M3SubtitleAutomationRunner
    {
        private const int MaxSearchCandidates = 20;
        private const int HardMaxCandidateFetches = 3;
        private const int MaxItemsPerRun = 100;
        private const int MaxSubtitleBytes = 16 * 1024 * 1024;

        private static readonly M1SubtitleValidator Validator = new M1SubtitleValidator();
        private static readonly M1SubtitleTimelineShifter TimelineShifter = new M1SubtitleTimelineShifter();
        private static readonly M2PreferenceAnalyzer PreferenceAnalyzer = new M2PreferenceAnalyzer();
        private static readonly M2PresenceAnalyzer PresenceAnalyzer = new M2PresenceAnalyzer();
        private static readonly M3AutomationPolicy Policy = new M3AutomationPolicy();
        private static readonly M3ReferenceSubtitleAligner ReferenceAligner = new M3ReferenceSubtitleAligner();

        private readonly ILibraryManager libraryManager;
        private readonly ISubtitleManager subtitleManager;
        private readonly IProviderManager providerManager;
        private readonly IFileSystem fileSystem;
        private readonly ILogger logger;
        private static readonly SemaphoreSlim RunGate = new SemaphoreSlim(1, 1);

        public M3SubtitleAutomationRunner(
            ILibraryManager libraryManager,
            ISubtitleManager subtitleManager,
            IProviderManager providerManager,
            IFileSystem fileSystem,
            ILogManager logManager = null)
        {
            this.libraryManager = libraryManager ?? throw new ArgumentNullException(nameof(libraryManager));
            this.subtitleManager = subtitleManager ?? throw new ArgumentNullException(nameof(subtitleManager));
            this.providerManager = providerManager ?? throw new ArgumentNullException(nameof(providerManager));
            this.fileSystem = fileSystem ?? throw new ArgumentNullException(nameof(fileSystem));
            try
            {
                logger = logManager?.GetLogger(nameof(M3SubtitleAutomationRunner));
            }
            catch
            {
                logger = null;
            }
        }

        public async Task<M3AutomationRunResult> RunAsync(
            PluginConfiguration configuration,
            CancellationToken cancellationToken,
            IProgress<double> progress = null,
            string itemId = null)
        {
            var result = new M3AutomationRunResult
            {
                StartedAtUtc = DateTime.UtcNow,
                DryRun = configuration?.AutomationDryRun ?? true
            };
            progress?.Report(0);

            if (configuration == null || !configuration.AutomationEnabled)
            {
                result.Status = M3AutomationResultNames.Skipped;
                result.Reasons.Add("M3 automation is disabled");
                CompleteRun(result, progress);
                return result;
            }

            var libraryTargets = ResolveAuthorizedLibraries(configuration.AutomationLibraryIds);
            result.LibraryCount = libraryTargets.Count;
            if (libraryTargets.Count == 0)
            {
                result.Status = M3AutomationResultNames.Skipped;
                result.Reasons.Add("M3 has no resolvable authorized media library");
                CompleteRun(result, progress);
                return result;
            }

            var maxItems = Clamp(configuration.AutomationMaxItemsPerRun, 1, MaxItemsPerRun, 20);
            var maxFetches = Clamp(configuration.AutomationMaxCandidateFetchesPerItem, 1, HardMaxCandidateFetches, HardMaxCandidateFetches);
            var workItems = QueryWorkItems(libraryTargets, maxItems, itemId);

            foreach (var workItem in workItems)
            {
                cancellationToken.ThrowIfCancellationRequested();
                result.ScannedCount++;

                M3AutomationItemResult itemResult;
                try
                {
                    itemResult = await ProcessItemAsync(workItem, configuration, maxFetches, cancellationToken).ConfigureAwait(false);
                }
                catch (OperationCanceledException)
                {
                    throw;
                }
                catch (Exception exception)
                {
                    itemResult = ItemResult(workItem.Item, workItem.LibraryName, M3AutomationResultNames.Failed, "Unexpected M3 item failure");
                    LogError("Automation item failed item=" + SafeLogText(workItem.Item?.Name), exception);
                }

                result.Items.Add(itemResult);
                CountResult(result, itemResult.Status);
                CountSynchronization(result, itemResult);
                progress?.Report(workItems.Count == 0 ? 1 : result.ScannedCount / (double)workItems.Count);
            }

            CompleteRun(result, progress);
            LogInfo(
                "Automation run status={0} statusCode={1} dryRun={2} libraries={3} scanned={4} 已完成={5} 已跳过={6} 失败={7} 需人工={8} 对轴通过={9} 对轴未知={10} 对轴漂移={11}",
                M3AutomationResultLabels.ForCode(result.Status),
                result.Status,
                result.DryRun,
                result.LibraryCount,
                result.ScannedCount,
                result.CompletedCount,
                result.SkippedCount,
                result.FailedCount,
                result.ManualCount,
                result.SynchronizationPassCount,
                result.SynchronizationUnknownCount,
                result.SynchronizationDriftCount);
            return result;
        }

        /// <summary>
        /// Serializes scheduled and API-triggered passes. A second run fails
        /// closed instead of interleaving provider calls or sidecar writes.
        /// </summary>
        public async Task<M3AutomationRunResult> RunExclusiveAsync(
            PluginConfiguration configuration,
            CancellationToken cancellationToken,
            IProgress<double> progress = null,
            string itemId = null)
        {
            if (!RunGate.Wait(0))
            {
                throw new InvalidOperationException("An M3 automation run is already in progress.");
            }

            try
            {
                var result = await RunAsync(configuration, cancellationToken, progress, itemId).ConfigureAwait(false);
                M3AutomationRunStore.Record(result);
                return result;
            }
            finally
            {
                RunGate.Release();
            }
        }

        private async Task<M3AutomationItemResult> ProcessItemAsync(
            M3WorkItem workItem,
            PluginConfiguration configuration,
            int maxFetches,
            CancellationToken cancellationToken)
        {
            M3ItemContext context;
            try
            {
                context = BuildContext(workItem.Item, configuration);
            }
            catch (Exception exception)
            {
                LogWarning("Automation item state unavailable item={0} exceptionType={1}", SafeLogText(workItem.Item?.Name), SafeExceptionType(exception));
                return ItemResult(workItem.Item, workItem.LibraryName, M3AutomationResultNames.Manual, "Required media/source state could not be read");
            }

            var eligibility = Policy.EvaluateItem(context.Eligibility);
            if (!eligibility.IsEligible)
            {
                return ItemResult(workItem.Item, workItem.LibraryName, ToItemStatus(eligibility.Decision), eligibility.Reasons.ToArray());
            }

            var itemIsStrm = IsStrmItem(workItem.Item);
            List<RemoteSubtitleInfo> candidates;
            try
            {
                candidates = (await subtitleManager.SearchSubtitles(
                        workItem.Item,
                        context.TargetLanguage.Code,
                        false,
                        false,
                        false,
                        cancellationToken).ConfigureAwait(false) ?? Array.Empty<RemoteSubtitleInfo>())
                    .Where(candidate => candidate != null && !string.IsNullOrWhiteSpace(candidate.Id))
                    .OrderByDescending(candidate => !itemIsStrm && candidate.IsHashMatch.GetValueOrDefault())
                    .ThenByDescending(candidate => CandidateMatchesItem(candidate.Name, workItem.Item))
                    .ThenByDescending(candidate => CandidateMatchesReleaseYear(candidate.Name, workItem.Item))
                    .ThenBy(candidate => M1CandidateEvidence.LooksLikeNonFullRelease(candidate.Name))
                    .Take(MaxSearchCandidates)
                    .ToList();
            }
            catch (OperationCanceledException)
            {
                throw;
            }
            catch (Exception exception)
            {
                LogError("Automation search failed item=" + SafeLogText(workItem.Item?.Name), exception);
                return ItemResult(workItem.Item, workItem.LibraryName, M3AutomationResultNames.Failed, "Provider search failed");
            }

            if (candidates.Count == 0)
            {
                return ItemResult(workItem.Item, workItem.LibraryName, M3AutomationResultNames.Skipped, "No subtitle candidate was returned");
            }

            var attempts = 0;
            var manualReviewNeeded = false;
            var providerFailure = false;
            var lastReason = "No candidate passed the M3 automatic gates";
            M3ReferenceAlignmentResult lastAlignment = null;
            var fetchedCandidates = new List<M3FetchedCandidate>();
            M3FetchedCandidate accepted = null;

            foreach (var candidate in candidates)
            {
                var hashMatch = candidate.IsHashMatch.GetValueOrDefault();
                var titleMatch = CandidateMatchesItem(candidate.Name, workItem.Item);
                var isStrm = itemIsStrm;
                var releaseYearMatch = CandidateMatchesReleaseYear(candidate.Name, workItem.Item);
                var episodeMatch = CandidateMatchesEpisode(candidate.Name, workItem.Item);
                var candidateLanguage = candidate.Language ?? context.TargetLanguage.Code;
                var parsedCandidateLanguage = M2Language.Parse(candidateLanguage, context.TargetLanguage.Code);
                var languageMismatch = !string.IsNullOrWhiteSpace(candidate.Language)
                    && !string.Equals(parsedCandidateLanguage.Code, context.TargetLanguage.Code, StringComparison.OrdinalIgnoreCase);
                var variantMismatch = !string.IsNullOrWhiteSpace(parsedCandidateLanguage.Variant)
                    && !string.IsNullOrWhiteSpace(context.TargetLanguage.Variant)
                    && !string.Equals(parsedCandidateLanguage.Variant, context.TargetLanguage.Variant, StringComparison.OrdinalIgnoreCase);
                var metadataGate = Policy.EvaluateCandidateMetadata(new M3CandidateMetadataInput
                {
                    HashMatch = hashMatch,
                    TitleMatch = titleMatch,
                    LikelyNonFullRelease = M1CandidateEvidence.LooksLikeNonFullRelease(candidate.Name),
                    LanguageMismatch = languageMismatch,
                    VariantMismatch = variantMismatch,
                    IsStrm = isStrm,
                    ReleaseYearMatch = releaseYearMatch,
                    EpisodeMatch = episodeMatch
                });

                if (!metadataGate.IsEligible)
                {
                    if (titleMatch && !hashMatch)
                    {
                        manualReviewNeeded = true;
                    }

                    lastReason = metadataGate.Reasons.LastOrDefault() ?? lastReason;
                    LogInfo("Automation candidate 已跳过 item={0} provider={1} reason={2}", SafeLogText(workItem.Item?.Name), SafeLogText(candidate.ProviderName), SafeLogText(lastReason));
                    continue;
                }

                if (attempts >= maxFetches)
                {
                    break;
                }

                attempts++;
                M3FetchedCandidate fetchedCandidate;
                try
                {
                    fetchedCandidate = await FetchCandidateAsync(
                        workItem.Item,
                        candidate,
                        context,
                        hashMatch,
                        titleMatch,
                        cancellationToken).ConfigureAwait(false);
                }
                catch (OperationCanceledException)
                {
                    throw;
                }
                catch (Exception exception)
                {
                    providerFailure = true;
                    lastReason = "Candidate Fetch failed";
                    LogWarning("Automation candidate Fetch failed item={0} provider={1} exceptionType={2}", SafeLogText(workItem.Item?.Name), SafeLogText(candidate.ProviderName), SafeExceptionType(exception));
                    continue;
                }

                var candidateGate = Policy.EvaluateCandidate(new M3CandidateGateInput
                {
                    HashMatch = hashMatch,
                    TitleMatch = titleMatch,
                    LikelyNonFullRelease = M1CandidateEvidence.LooksLikeNonFullRelease(candidate.Name),
                    LanguageMismatch = languageMismatch,
                    VariantMismatch = variantMismatch,
                    IsStrm = isStrm,
                    ReleaseYearMatch = releaseYearMatch,
                    EpisodeMatch = episodeMatch,
                    Health = fetchedCandidate.Validation.Health,
                    TargetLanguagePresent = fetchedCandidate.Preference.Quality != null && fetchedCandidate.Preference.Quality.TargetLanguagePresent,
                    PreferenceSuitability = fetchedCandidate.Preference.Suitability,
                    PreferBilingual = context.PreferenceOptions.PreferBilingual,
                    BilingualDetected = fetchedCandidate.Preference.Quality != null && fetchedCandidate.Preference.Quality.BilingualDetected,
                    BilingualConfidence = fetchedCandidate.Preference.Quality == null ? 0d : fetchedCandidate.Preference.Quality.BilingualConfidence
                });
                fetchedCandidate.HashMatch = hashMatch;
                fetchedCandidate.TitleMatch = titleMatch;
                fetchedCandidate.CandidateLanguage = candidateLanguage;
                fetchedCandidate.Gate = candidateGate;
                fetchedCandidates.Add(fetchedCandidate);
                lastReason = candidateGate.Reasons.LastOrDefault() ?? lastReason;
                if (!candidateGate.IsEligible)
                {
                    manualReviewNeeded |= string.Equals(candidateGate.Decision, M3AutomationDecisionNames.Manual, StringComparison.Ordinal);
                    LogInfo("Automation candidate rejected item={0} provider={1} health={2} preference={3} reason={4}", SafeLogText(workItem.Item?.Name), SafeLogText(candidate.ProviderName), fetchedCandidate.Validation.Health, fetchedCandidate.Preference.Suitability, SafeLogText(lastReason));
                    continue;
                }

                LogInfo("Automation candidate已抓取待对轴 item={0} provider={1} health={2} preference={3} matchScore={4}", SafeLogText(workItem.Item?.Name), SafeLogText(candidate.ProviderName), fetchedCandidate.Validation.Health, fetchedCandidate.Preference.Suitability, candidateGate.MatchScore);
            }

            if (fetchedCandidates.Any(candidate => candidate.Gate != null && candidate.Gate.IsEligible))
            {
                var consensus = ReferenceAligner.FindConsensus(
                    fetchedCandidates
                        .Select(candidate => new M3CandidateAlignmentInput
                        {
                            Validation = candidate.Validation,
                            Language = candidate.CandidateLanguage,
                            InstallEligible = candidate.Gate != null && candidate.Gate.IsEligible,
                            TargetLanguagePresent = candidate.Preference.Quality != null && candidate.Preference.Quality.TargetLanguagePresent,
                            PreferenceScore = candidate.Preference.Score
                        })
                        .ToList());
                lastAlignment = consensus.Alignment;
                lastReason = consensus.Reasons.LastOrDefault() ?? lastReason;
                if (consensus.IsAligned
                    && consensus.SelectedCandidateIndex >= 0
                    && consensus.SelectedCandidateIndex < fetchedCandidates.Count)
                {
                    accepted = fetchedCandidates[consensus.SelectedCandidateIndex];
                    accepted.Synchronization = consensus.Alignment;
                    if (!ApplyReferenceAlignment(accepted, context, accepted.HashMatch, accepted.TitleMatch, consensus.Alignment))
                    {
                        manualReviewNeeded = true;
                        lastReason = "对轴后的字幕未通过重新校验";
                        LogInfo("Automation candidate需人工复核 item={0} reason={1}", SafeLogText(workItem.Item?.Name), lastReason);
                        accepted = null;
                    }
                    else
                    {
                        LogInfo("Automation candidate已通过 item={0} health={1} preference={2} matchScore={3} syncMethod={4} syncMatches={5} syncCoverage={6:F3} syncOffset={7}", SafeLogText(workItem.Item?.Name), SafeLogText(accepted.Validation.Health), SafeLogText(accepted.Preference.Suitability), accepted.Gate.MatchScore, SafeLogText(consensus.Alignment.Method), consensus.Alignment.MatchCount, consensus.Alignment.Coverage, consensus.Alignment.OffsetMilliseconds);
                    }
                }
                else
                {
                    manualReviewNeeded = true;
                    LogInfo("Automation candidate需人工对轴 item={0} syncStatus={1} reason={2}", SafeLogText(workItem.Item?.Name), SafeLogText(consensus.Status), SafeLogText(lastReason));
                }
            }

            if (accepted == null)
            {
                var status = providerFailure && attempts > 0 && !manualReviewNeeded
                    ? M3AutomationResultNames.Failed
                    : manualReviewNeeded ? M3AutomationResultNames.Manual : M3AutomationResultNames.Skipped;
                var rejectedResult = ItemResult(workItem.Item, workItem.LibraryName, status, attempts, lastReason);
                CopySynchronizationResult(rejectedResult, lastAlignment);
                return rejectedResult;
            }

            if (configuration.AutomationDryRun)
            {
                var dryRunResult = ItemResult(
                    workItem.Item,
                    workItem.LibraryName,
                    M3AutomationResultNames.Skipped,
                    attempts,
                    "Dry-run validated a candidate; no media file was written");
                CopySynchronizationResult(dryRunResult, accepted);
                return dryRunResult;
            }

            var installResult = await InstallCandidateAsync(
                workItem.Item,
                workItem.LibraryName,
                context,
                accepted,
                attempts,
                cancellationToken).ConfigureAwait(false);
            CopySynchronizationResult(installResult, accepted);
            return installResult;
        }

        private async Task<M3FetchedCandidate> FetchCandidateAsync(
            BaseItem item,
            RemoteSubtitleInfo candidate,
            M3ItemContext context,
            bool hashMatch,
            bool titleMatch,
            CancellationToken cancellationToken)
        {
            var fetched = await subtitleManager.GetRemoteSubtitles(candidate.Id, cancellationToken).ConfigureAwait(false);
            if (fetched == null || fetched.Stream == null)
            {
                throw new InvalidOperationException("Emby returned no subtitle content");
            }

            byte[] content;
            using (fetched.Stream)
            {
                content = await ReadBoundedAsync(fetched.Stream, cancellationToken).ConfigureAwait(false);
            }

            var validation = Validator.Validate(content, fetched.Format);
            var preference = PreferenceAnalyzer.Evaluate(
                validation,
                context.TargetLanguage.Code,
                context.SecondaryLanguage.Code,
                null,
                titleMatch,
                hashMatch && !IsStrmItem(item),
                context.PreferenceOptions);
            return new M3FetchedCandidate
            {
                Content = content,
                Validation = validation,
                Preference = preference,
                RequestedLanguageVariant = context.TargetLanguage.Variant,
                Language = context.TargetLanguage.Code,
                CandidateLanguage = candidate.Language ?? context.TargetLanguage.Code,
                IsStrm = IsStrmItem(item)
            };
        }

        private static int NormalizeTimelineOffset(int offsetMilliseconds, string format)
        {
            var normalizedFormat = (format ?? string.Empty).Trim().TrimStart('.').ToLowerInvariant();
            if (normalizedFormat == "ass" || normalizedFormat == "ssa")
            {
                return Convert.ToInt32(Math.Round(offsetMilliseconds / 10d, MidpointRounding.AwayFromZero)) * 10;
            }

            return offsetMilliseconds;
        }

        private bool ApplyReferenceAlignment(
            M3FetchedCandidate candidate,
            M3ItemContext context,
            bool hashMatch,
            bool titleMatch,
            M3ReferenceAlignmentResult alignment)
        {
            var offset = NormalizeTimelineOffset(alignment.OffsetMilliseconds, candidate.Validation.Format);
            alignment.OffsetMilliseconds = offset;
            candidate.TimelineOffsetMilliseconds = offset;
            if (offset == 0)
            {
                return true;
            }

            try
            {
                var shiftedContent = TimelineShifter.Shift(candidate.Content, candidate.Validation.Format, offset);
                var shiftedValidation = Validator.Validate(shiftedContent, candidate.Validation.Format);
                if (!string.Equals(shiftedValidation.Health, "PASS", StringComparison.OrdinalIgnoreCase))
                {
                    return false;
                }

                var shiftedPreference = PreferenceAnalyzer.Evaluate(
                    shiftedValidation,
                    context.TargetLanguage.Code,
                    context.SecondaryLanguage.Code,
                    null,
                    titleMatch,
                    hashMatch && !candidate.IsStrm,
                    context.PreferenceOptions);
                if (shiftedPreference.Quality == null
                    || !shiftedPreference.Quality.TargetLanguagePresent
                    || !string.Equals(shiftedPreference.Suitability, "RECOMMENDED", StringComparison.OrdinalIgnoreCase))
                {
                    return false;
                }

                candidate.Content = shiftedContent;
                candidate.Validation = shiftedValidation;
                candidate.Preference = shiftedPreference;
                return true;
            }
            catch (ArgumentException)
            {
                return false;
            }
            catch (InvalidOperationException)
            {
                return false;
            }
            catch (NotSupportedException)
            {
                return false;
            }
        }

        private async Task<M3AutomationItemResult> InstallCandidateAsync(
            BaseItem item,
            string libraryName,
            M3ItemContext context,
            M3FetchedCandidate candidate,
            int attempts,
            CancellationToken cancellationToken)
        {
            string targetPath = null;
            try
            {
                targetPath = WriteSidecar(item, context.Source, candidate);
                await providerManager.RefreshFullItem(
                    item,
                    new MetadataRefreshOptions(new DirectoryService(fileSystem)),
                    cancellationToken).ConfigureAwait(false);
                var refreshed = libraryManager.GetItemById(item.Id);
                if (refreshed == null || !HasExternalSubtitleStream(refreshed, context.Source.Id, targetPath))
                {
                    throw new InvalidOperationException("Emby did not report the newly installed subtitle stream after refresh");
                }

                LogInfo("Automation install success item={0} format={1} fileName={2}", SafeLogText(item.Name), SafeLogText(candidate.Validation.Format), SafeLogText(Path.GetFileName(targetPath)));
                return ItemResult(item, libraryName, M3AutomationResultNames.Completed, attempts, "M3 installed and confirmed the new subtitle stream", Path.GetFileName(targetPath));
            }
            catch (OperationCanceledException)
            {
                if (!string.IsNullOrWhiteSpace(targetPath))
                {
                    TryDeleteCreatedFile(targetPath);
                }

                throw;
            }
            catch (Exception exception)
            {
                if (!string.IsNullOrWhiteSpace(targetPath))
                {
                    TryDeleteCreatedFile(targetPath);
                }

                LogError("Automation install failed item=" + SafeLogText(item.Name), exception);
                return ItemResult(item, libraryName, M3AutomationResultNames.Failed, attempts, "Install or MediaStream confirmation failed");
            }
        }

        private M3ItemContext BuildContext(BaseItem item, PluginConfiguration configuration)
        {
            var sources = (item.GetMediaSources(false, false, new LibraryOptions()) ?? new List<MediaSourceInfo>()).ToList();
            var libraryOverride = GetLibraryOverride(item, configuration);
            var targetLanguage = M2Options.ParseTargetLanguage(
                libraryOverride != null && libraryOverride.Enabled && !string.IsNullOrWhiteSpace(libraryOverride.TargetLanguage)
                    ? libraryOverride.TargetLanguage
                    : configuration.TargetLanguage);
            var secondaryLanguage = M2Options.ParseSecondaryLanguage(
                libraryOverride != null && libraryOverride.Enabled && !string.IsNullOrWhiteSpace(libraryOverride.SecondaryLanguage)
                    ? libraryOverride.SecondaryLanguage
                    : configuration.SecondaryLanguage);
            var preferenceOptions = M2Options.ParsePreferenceOptions(
                libraryOverride != null && libraryOverride.Enabled ? libraryOverride.PreferBilingual : configuration.PreferBilingual,
                libraryOverride != null && libraryOverride.Enabled && !string.IsNullOrWhiteSpace(libraryOverride.FormatOrder)
                    ? libraryOverride.FormatOrder
                    : configuration.FormatOrder);

            var context = new M3ItemContext
            {
                Sources = sources,
                TargetLanguage = targetLanguage,
                SecondaryLanguage = secondaryLanguage,
                PreferenceOptions = preferenceOptions,
                Eligibility = new M3EligibilityInput
                {
                    AutomationEnabled = configuration.AutomationEnabled,
                    LibraryAuthorized = true,
                    SourceCount = sources.Count,
                    StateKnown = true
                }
            };

            if (sources.Count == 1)
            {
                context.Source = sources[0];
                var subtitleStreams = (context.Source.MediaStreams ?? new List<MediaStream>())
                    .Where(stream => stream.Type.ToString() == "Subtitle")
                    .Select(stream => new M2SubtitleStreamSnapshot
                    {
                        IsExternal = stream.IsExternal,
                        Language = stream.Language,
                        Title = string.IsNullOrWhiteSpace(stream.DisplayTitle) ? stream.Title : stream.DisplayTitle,
                        Path = stream.IsExternal ? stream.Path : null
                    })
                    .ToList();
                context.Presence = PresenceAnalyzer.Analyze(
                    subtitleStreams,
                    targetLanguage.Variant ?? targetLanguage.Code,
                    secondaryLanguage.Variant ?? secondaryLanguage.Code);
                context.Eligibility.TargetLanguagePresent = context.Presence.TargetLanguagePresent;
                context.Eligibility.HasSourceIdentity = !string.IsNullOrWhiteSpace(context.Source.Id);
                context.Eligibility.HasSafeWriteAnchor = HasSafeWriteAnchor(item, context.Source);
            }

            return context;
        }

        private List<M3WorkItem> QueryWorkItems(IReadOnlyList<M3LibraryTarget> targets, int maxItems, string itemId)
        {
            var workItems = new List<M3WorkItem>();
            var seen = new HashSet<Guid>();

            if (!string.IsNullOrWhiteSpace(itemId))
            {
                if (!Guid.TryParse(itemId, out var requestedId))
                {
                    throw new ArgumentException("ItemId is invalid.");
                }

                var requestedItem = libraryManager.GetItemById(requestedId);
                if (requestedItem == null
                    || (!string.Equals(requestedItem.GetType().Name, "Movie", StringComparison.Ordinal)
                        && !string.Equals(requestedItem.GetType().Name, "Episode", StringComparison.Ordinal)))
                {
                    throw new ArgumentException("ItemId is not a supported Movie or Episode.");
                }

                var authorizedLibrary = FindAuthorizedLibrary(requestedItem, targets);
                if (authorizedLibrary == null)
                {
                    throw new ArgumentException("ItemId is not inside an authorized M3 media library.");
                }

                workItems.Add(new M3WorkItem
                {
                    Item = requestedItem,
                    LibraryName = authorizedLibrary.Name
                });
                return workItems;
            }

            foreach (var target in targets)
            {
                var remaining = maxItems - workItems.Count;
                if (remaining <= 0)
                {
                    break;
                }

                var query = new InternalItemsQuery
                {
                    AncestorIds = new[] { target.InternalId },
                    IncludeItemTypes = new[] { "Movie", "Episode" },
                    Recursive = true,
                    Limit = remaining
                };
                var items = libraryManager.GetItemList(query) ?? Array.Empty<BaseItem>();
                foreach (var item in items)
                {
                    if (item != null && seen.Add(item.Id))
                    {
                        workItems.Add(new M3WorkItem { Item = item, LibraryName = target.Name });
                    }
                }
            }

            return workItems;
        }

        private M3LibraryTarget FindAuthorizedLibrary(BaseItem item, IReadOnlyList<M3LibraryTarget> targets)
        {
            var folders = libraryManager.GetCollectionFolders(item);
            if (folders == null)
            {
                return null;
            }

            foreach (var folder in folders)
            {
                if (folder == null)
                {
                    continue;
                }

                var folderId = NormalizeLibraryId(folder.Id.ToString("N"));
                var target = targets.FirstOrDefault(candidate => candidate.InternalId == folder.InternalId
                    || string.Equals(candidate.Id, folderId, StringComparison.OrdinalIgnoreCase));
                if (target != null)
                {
                    return target;
                }
            }

            return null;
        }

        private List<M3LibraryTarget> ResolveAuthorizedLibraries(IEnumerable<string> configuredIds)
        {
            var requestedIds = new HashSet<string>(
                (configuredIds ?? Array.Empty<string>())
                    .Where(value => !string.IsNullOrWhiteSpace(value))
                    .Select(NormalizeLibraryId),
                StringComparer.OrdinalIgnoreCase);
            var targets = new List<M3LibraryTarget>();
            if (requestedIds.Count == 0)
            {
                return targets;
            }

            var folders = libraryManager.GetVirtualFolders();
            if (folders == null)
            {
                return targets;
            }

            foreach (var folder in folders)
            {
                if (folder == null || string.IsNullOrWhiteSpace(folder.ItemId))
                {
                    continue;
                }

                var normalizedId = NormalizeLibraryId(folder.ItemId);
                if (!requestedIds.Contains(normalizedId))
                {
                    continue;
                }

                BaseItem library = null;
                if (long.TryParse(folder.ItemId, out var internalId))
                {
                    library = libraryManager.GetItemById(internalId);
                }
                else if (Guid.TryParse(folder.ItemId, out var parsedId))
                {
                    library = libraryManager.GetItemById(parsedId);
                }

                if (library == null)
                {
                    continue;
                }

                targets.Add(new M3LibraryTarget
                {
                    Id = normalizedId,
                    Name = folder.Name,
                    InternalId = library.InternalId
                });
            }

            return targets
                .GroupBy(target => target.Id, StringComparer.OrdinalIgnoreCase)
                .Select(group => group.First())
                .ToList();
        }

        private LibraryPreferenceOverride GetLibraryOverride(BaseItem item, PluginConfiguration configuration)
        {
            var folders = libraryManager.GetCollectionFolders(item);
            var folder = folders == null ? null : folders.FirstOrDefault();
            if (folder == null || configuration?.LibraryOverrides == null)
            {
                return null;
            }

            var libraryId = folder.Id.ToString("N");
            return configuration.LibraryOverrides.FirstOrDefault(entry =>
                entry != null && string.Equals(NormalizeLibraryId(entry.LibraryId), libraryId, StringComparison.OrdinalIgnoreCase));
        }

        private static bool HasSafeWriteAnchor(BaseItem item, MediaSourceInfo source)
        {
            var anchor = item != null && !string.IsNullOrWhiteSpace(item.Path) && item.Path.EndsWith(".strm", StringComparison.OrdinalIgnoreCase)
                ? item.Path
                : source == null || source.IsRemote ? null : source.Path;
            if (!IsRegularFile(anchor))
            {
                return false;
            }

            try
            {
                var directory = Path.GetDirectoryName(Path.GetFullPath(anchor));
                if (string.IsNullOrWhiteSpace(directory) || !Directory.Exists(directory))
                {
                    return false;
                }

                var attributes = File.GetAttributes(directory);
                return (attributes & FileAttributes.Directory) != 0
                    && (attributes & FileAttributes.ReparsePoint) == 0;
            }
            catch (ArgumentException)
            {
                return false;
            }
            catch (IOException)
            {
                return false;
            }
            catch (UnauthorizedAccessException)
            {
                return false;
            }
        }

        private static bool IsRegularFile(string path)
        {
            if (string.IsNullOrWhiteSpace(path) || !File.Exists(path))
            {
                return false;
            }

            try
            {
                var attributes = File.GetAttributes(path);
                return (attributes & FileAttributes.Directory) == 0
                    && (attributes & FileAttributes.ReparsePoint) == 0;
            }
            catch (IOException)
            {
                return false;
            }
            catch (UnauthorizedAccessException)
            {
                return false;
            }
        }

        private static string WriteSidecar(BaseItem item, MediaSourceInfo source, M3FetchedCandidate candidate)
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
                    throw new InvalidOperationException("The selected non-STRM source is not a local filesystem path");
                }

                anchor = source.Path;
                EnsureRegularFile(anchor, "MediaSource.Path");
            }

            var directory = Path.GetDirectoryName(Path.GetFullPath(anchor));
            var baseName = Path.GetFileNameWithoutExtension(anchor);
            if (string.IsNullOrWhiteSpace(directory) || string.IsNullOrWhiteSpace(baseName))
            {
                throw new InvalidOperationException("The selected media path cannot produce a sidecar target");
            }

            var directoryAttributes = File.GetAttributes(directory);
            if ((directoryAttributes & FileAttributes.Directory) == 0 || (directoryAttributes & FileAttributes.ReparsePoint) != 0)
            {
                throw new InvalidOperationException("The selected media directory is not a regular directory");
            }

            var extension = candidate.Validation.Format == "ass" || candidate.Validation.Format == "ssa" ? candidate.Validation.Format : "srt";
            var languageTag = M2Options.ResolveSubtitleLanguageTag(candidate.RequestedLanguageVariant, candidate.Language);
            var contentTypeLabel = M2SidecarLabel.Build(candidate.Validation, candidate.RequestedLanguageVariant);
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
                        output.Write(candidate.Content, 0, candidate.Content.Length);
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

            throw new IOException("Unable to allocate a new subtitle sidecar filename");
        }

        private static void EnsureRegularFile(string path, string label)
        {
            if (!IsRegularFile(path))
            {
                throw new InvalidOperationException(label + " does not point to a regular file");
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
                .SelectMany(source => source.MediaStreams ?? new List<MediaStream>())
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

        private static async Task<byte[]> ReadBoundedAsync(Stream stream, CancellationToken cancellationToken)
        {
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

                    if (output.Length + read > MaxSubtitleBytes)
                    {
                        throw new InvalidOperationException("Fetched subtitle exceeds the M3 size limit");
                    }

                    output.Write(buffer, 0, read);
                }

                return output.ToArray();
            }
        }

        private static void CompleteRun(M3AutomationRunResult result, IProgress<double> progress)
        {
            result.CompletedAtUtc = DateTime.UtcNow;
            progress?.Report(1);
        }

        private static void CountResult(M3AutomationRunResult result, string status)
        {
            if (string.Equals(status, M3AutomationResultNames.Completed, StringComparison.Ordinal))
            {
                result.CompletedCount++;
            }
            else if (string.Equals(status, M3AutomationResultNames.Failed, StringComparison.Ordinal))
            {
                result.FailedCount++;
            }
            else if (string.Equals(status, M3AutomationResultNames.Manual, StringComparison.Ordinal))
            {
                result.ManualCount++;
            }
            else
            {
                result.SkippedCount++;
            }
        }

        private static void CountSynchronization(M3AutomationRunResult result, M3AutomationItemResult itemResult)
        {
            if (result == null || itemResult == null || string.IsNullOrWhiteSpace(itemResult.SynchronizationStatus))
            {
                return;
            }

            if (string.Equals(itemResult.SynchronizationStatus, M3ReferenceSyncNames.Pass, StringComparison.Ordinal))
            {
                result.SynchronizationPassCount++;
                result.LastSynchronizationMethod = itemResult.SynchronizationMethod;
                result.LastTimelineOffsetMilliseconds = itemResult.TimelineOffsetMilliseconds;
                result.LastSynchronizationReason = itemResult.SynchronizationReason;
            }
            else if (string.Equals(itemResult.SynchronizationStatus, M3ReferenceSyncNames.Drift, StringComparison.Ordinal))
            {
                result.SynchronizationDriftCount++;
                result.LastSynchronizationReason = itemResult.SynchronizationReason;
            }
            else if (string.Equals(itemResult.SynchronizationStatus, M3ReferenceSyncNames.Unknown, StringComparison.Ordinal))
            {
                result.SynchronizationUnknownCount++;
                result.LastSynchronizationReason = itemResult.SynchronizationReason;
            }
        }

        private static string ToItemStatus(string decision)
        {
            return string.Equals(decision, M3AutomationDecisionNames.Manual, StringComparison.Ordinal)
                ? M3AutomationResultNames.Manual
                : M3AutomationResultNames.Skipped;
        }

        private static M3AutomationItemResult ItemResult(BaseItem item, string libraryName, string status, params string[] reasons)
        {
            var result = new M3AutomationItemResult
            {
                ItemId = item?.Id.ToString("N"),
                ItemName = item?.Name,
                LibraryName = libraryName,
                Status = status
            };
            if (reasons != null)
            {
                result.Reasons.AddRange(reasons.Where(reason => !string.IsNullOrWhiteSpace(reason)));
            }

            return result;
        }

        private static M3AutomationItemResult ItemResult(BaseItem item, string libraryName, string status, int attempts, string reason, string fileName = null)
        {
            var result = ItemResult(item, libraryName, status, reason);
            result.CandidateAttempts = attempts;
            result.FileName = fileName;
            return result;
        }

        private static int Clamp(int value, int minimum, int maximum, int fallback)
        {
            if (value < minimum)
            {
                return fallback;
            }

            return Math.Min(value, maximum);
        }

        private static string NormalizeLibraryId(string libraryId)
        {
            if (Guid.TryParse(libraryId, out var parsed))
            {
                return parsed.ToString("N");
            }

            return libraryId?.Trim();
        }

        private static bool CandidateMatchesItem(string candidateName, BaseItem item)
        {
            if (string.IsNullOrWhiteSpace(candidateName) || item == null)
            {
                return false;
            }

            return (!string.IsNullOrWhiteSpace(item.Name) && candidateName.IndexOf(item.Name, StringComparison.OrdinalIgnoreCase) >= 0)
                || (!string.IsNullOrWhiteSpace(item.OriginalTitle) && candidateName.IndexOf(item.OriginalTitle, StringComparison.OrdinalIgnoreCase) >= 0)
                || IsSeriesTitleMatch(candidateName, item);
        }

        private static bool IsSeriesTitleMatch(string candidateName, BaseItem item)
        {
            var episode = item as Episode;
            var series = episode?.Series;
            return series != null
                && ((!string.IsNullOrWhiteSpace(series.Name) && candidateName.IndexOf(series.Name, StringComparison.OrdinalIgnoreCase) >= 0)
                    || (!string.IsNullOrWhiteSpace(series.OriginalTitle) && candidateName.IndexOf(series.OriginalTitle, StringComparison.OrdinalIgnoreCase) >= 0));
        }

        private static bool IsStrmItem(BaseItem item)
        {
            return item != null
                && !string.IsNullOrWhiteSpace(item.Path)
                && item.Path.EndsWith(".strm", StringComparison.OrdinalIgnoreCase);
        }

        private static bool CandidateMatchesReleaseYear(string candidateName, BaseItem item)
        {
            if (string.IsNullOrWhiteSpace(candidateName) || item == null || !item.ProductionYear.HasValue)
            {
                return false;
            }

            var year = item.ProductionYear.Value.ToString(System.Globalization.CultureInfo.InvariantCulture);
            var start = 0;
            while (true)
            {
                var index = candidateName.IndexOf(year, start, StringComparison.OrdinalIgnoreCase);
                if (index < 0)
                {
                    return false;
                }

                var beforeIsDigit = index > 0 && char.IsDigit(candidateName[index - 1]);
                var afterIndex = index + year.Length;
                var afterIsDigit = afterIndex < candidateName.Length && char.IsDigit(candidateName[afterIndex]);
                if (!beforeIsDigit && !afterIsDigit)
                {
                    return true;
                }

                start = afterIndex;
            }
        }

        private static bool CandidateMatchesEpisode(string candidateName, BaseItem item)
        {
            if (string.IsNullOrWhiteSpace(candidateName)
                || item == null
                || !string.Equals(item.GetType().Name, "Episode", StringComparison.Ordinal)
                || !item.IndexNumber.HasValue)
            {
                return false;
            }

            var episode = item.IndexNumber.Value;
            var episodeNumber = episode.ToString(System.Globalization.CultureInfo.InvariantCulture);
            var episodeNumberPadded = episode.ToString("00");
            if (item.ParentIndexNumber.HasValue)
            {
                var season = item.ParentIndexNumber.Value;
                var seasonNumber = season.ToString(System.Globalization.CultureInfo.InvariantCulture);
                var seasonNumberPadded = season.ToString("00");
                return ContainsPattern(candidateName, "s" + seasonNumberPadded + "e" + episodeNumberPadded)
                    || ContainsPattern(candidateName, "s" + seasonNumber + "e" + episodeNumber)
                    || ContainsPattern(candidateName, seasonNumber + "x" + episodeNumberPadded)
                    || ContainsPattern(candidateName, seasonNumber + "x" + episodeNumber);
            }

            return ContainsPattern(candidateName, "ep" + episodeNumberPadded)
                || ContainsPattern(candidateName, "ep" + episodeNumber)
                || ContainsPattern(candidateName, "episode " + episodeNumber);
        }

        private static bool ContainsPattern(string value, string pattern)
        {
            if (string.IsNullOrWhiteSpace(value) || string.IsNullOrWhiteSpace(pattern))
            {
                return false;
            }

            var start = 0;
            while (true)
            {
                var index = value.IndexOf(pattern, start, StringComparison.OrdinalIgnoreCase);
                if (index < 0)
                {
                    return false;
                }

                var beforeIsAlphaNumeric = index > 0 && char.IsLetterOrDigit(value[index - 1]);
                var afterIndex = index + pattern.Length;
                var afterIsAlphaNumeric = afterIndex < value.Length && char.IsLetterOrDigit(value[afterIndex]);
                if (!beforeIsAlphaNumeric && !afterIsAlphaNumeric)
                {
                    return true;
                }

                start = afterIndex;
            }
        }

        private static string SafeLogText(string value)
        {
            if (string.IsNullOrWhiteSpace(value))
            {
                return "(none)";
            }

            var normalized = value.Replace("\r", " ").Replace("\n", " ").Trim();
            return normalized.Length <= 120 ? normalized : normalized.Substring(0, 120) + "…";
        }

        private static string SafeExceptionType(Exception exception)
        {
            return exception == null ? "Unknown" : exception.GetType().Name;
        }

        private void LogInfo(string message, params object[] parameters)
        {
            try
            {
                logger?.Info("[SubSteward] " + message, parameters);
            }
            catch
            {
            }
        }

        private void LogWarning(string message, params object[] parameters)
        {
            try
            {
                logger?.Warn("[SubSteward] " + message, parameters);
            }
            catch
            {
            }
        }

        private void LogError(string message, Exception exception)
        {
            try
            {
                logger?.ErrorException("[SubSteward] " + message, exception);
            }
            catch
            {
            }
        }

        private sealed class M3LibraryTarget
        {
            public string Id { get; set; }

            public string Name { get; set; }

            public long InternalId { get; set; }
        }

        private sealed class M3WorkItem
        {
            public BaseItem Item { get; set; }

            public string LibraryName { get; set; }
        }

        private sealed class M3ItemContext
        {
            public List<MediaSourceInfo> Sources { get; set; }

            public MediaSourceInfo Source { get; set; }

            public M2PresenceReport Presence { get; set; }

            public M2LanguageSelection TargetLanguage { get; set; }

            public M2LanguageSelection SecondaryLanguage { get; set; }

            public M2PreferenceOptions PreferenceOptions { get; set; }

            public M3EligibilityInput Eligibility { get; set; }
        }

        private sealed class M3FetchedCandidate
        {
            public byte[] Content { get; set; }

            public M1ValidationResult Validation { get; set; }

            public M2PreferenceReport Preference { get; set; }

            public string Language { get; set; }

            public string RequestedLanguageVariant { get; set; }

            public string CandidateLanguage { get; set; }

            public bool IsStrm { get; set; }

            public bool HashMatch { get; set; }

            public bool TitleMatch { get; set; }

            public M3CandidateGateReport Gate { get; set; }

            public M3ReferenceAlignmentResult Synchronization { get; set; }

            public int TimelineOffsetMilliseconds { get; set; }
        }

        private static void CopySynchronizationResult(M3AutomationItemResult result, M3FetchedCandidate candidate)
        {
            if (candidate == null)
            {
                return;
            }

            CopySynchronizationResult(result, candidate.Synchronization, candidate.TimelineOffsetMilliseconds);
        }

        private static void CopySynchronizationResult(M3AutomationItemResult result, M3ReferenceAlignmentResult alignment)
        {
            CopySynchronizationResult(result, alignment, alignment?.OffsetMilliseconds ?? 0);
        }

        private static void CopySynchronizationResult(M3AutomationItemResult result, M3ReferenceAlignmentResult alignment, int offsetMilliseconds)
        {
            if (result == null || alignment == null)
            {
                return;
            }

            result.SynchronizationStatus = alignment.Status;
            result.SynchronizationMethod = alignment.Method;
            result.SynchronizationReason = alignment.Reasons.LastOrDefault();
            result.TimelineOffsetMilliseconds = offsetMilliseconds;
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
