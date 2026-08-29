using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using MediaBrowser.Controller.Api;
using MediaBrowser.Controller.Library;
using MediaBrowser.Controller.Net;
using MediaBrowser.Controller.Providers;
using MediaBrowser.Controller.Subtitles;
using MediaBrowser.Model.IO;
using MediaBrowser.Model.Logging;
using MediaBrowser.Model.Services;

namespace SubSteward.Plugin.M3
{
    [Route("/SubSteward/Automation", "GET", Summary = "Gets the M3 automation configuration and latest run summary")]
    [Authenticated(Roles = "Admin")]
    public sealed class M3AutomationStatusRequest
    {
    }

    [Route("/SubSteward/Automation/Run", "POST", Summary = "Runs one explicitly requested M3 automation pass")]
    [Authenticated(Roles = "Admin")]
    public sealed class M3AutomationRunRequest
    {
        /// <summary>
        /// Optional one-time item filter. The runner verifies that the item is
        /// a Movie/Episode inside the persisted automation allowlist.
        /// </summary>
        public string ItemId { get; set; }

        /// <summary>
        /// Optional per-run dry-run override. A false value cannot override a
        /// persisted true setting, so an administrator must explicitly change
        /// the persisted configuration before allowing writes.
        /// </summary>
        public bool? DryRun { get; set; }
    }

    public sealed class M3AutomationStatusResponse
    {
        public bool AutomationEnabled { get; set; }

        public bool AutomationDryRun { get; set; }

        public List<string> AutomationLibraryIds { get; } = new List<string>();

        public int AutomationMaxItemsPerRun { get; set; }

        public int AutomationMaxCandidateFetchesPerItem { get; set; }

        public M3AutomationRunSummary LastRun { get; set; }
    }

    /// <summary>
    /// Small API surface for API-first M3 operation while the embedded UI is
    /// still being repaired. It never widens the runner's persisted gates.
    /// </summary>
    public sealed class M3AutomationService : BaseApiService
    {
        private readonly ILibraryManager libraryManager;
        private readonly ISubtitleManager subtitleManager;
        private readonly IProviderManager providerManager;
        private readonly IFileSystem fileSystem;
        private readonly ILogManager logManager;

        public M3AutomationService(
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
            this.logManager = logManager;
        }

        public object Get(M3AutomationStatusRequest request)
        {
            return BuildStatus(Plugin.Instance.Configuration);
        }

        public async Task<object> Post(M3AutomationRunRequest request)
        {
            var persisted = Plugin.Instance.Configuration;
            if (persisted == null || !persisted.AutomationEnabled)
            {
                throw new InvalidOperationException("M3 automation is disabled in the persisted configuration.");
            }

            if (request?.DryRun == false && persisted.AutomationDryRun)
            {
                throw new InvalidOperationException("A non-dry M3 run requires AutomationDryRun=false in the persisted configuration.");
            }

            var effective = CopyConfiguration(persisted, request?.DryRun);
            var runner = new M3SubtitleAutomationRunner(
                libraryManager,
                subtitleManager,
                providerManager,
                fileSystem,
                logManager);
            await runner.RunExclusiveAsync(effective, Request.CancellationToken, null, request?.ItemId).ConfigureAwait(false);
            return new M3AutomationStatusResponse
            {
                AutomationEnabled = effective.AutomationEnabled,
                AutomationDryRun = effective.AutomationDryRun,
                AutomationMaxItemsPerRun = effective.AutomationMaxItemsPerRun,
                AutomationMaxCandidateFetchesPerItem = effective.AutomationMaxCandidateFetchesPerItem,
                LastRun = M3AutomationRunStore.GetLatest()
            }.WithLibraryIds(effective.AutomationLibraryIds);
        }

        private static M3AutomationStatusResponse BuildStatus(PluginConfiguration configuration)
        {
            var response = new M3AutomationStatusResponse
            {
                AutomationEnabled = configuration?.AutomationEnabled ?? false,
                AutomationDryRun = configuration?.AutomationDryRun ?? true,
                AutomationMaxItemsPerRun = configuration?.AutomationMaxItemsPerRun ?? 20,
                AutomationMaxCandidateFetchesPerItem = configuration?.AutomationMaxCandidateFetchesPerItem ?? 3,
                LastRun = M3AutomationRunStore.GetLatest()
            };
            return response.WithLibraryIds(configuration?.AutomationLibraryIds);
        }

        private static PluginConfiguration CopyConfiguration(PluginConfiguration source, bool? dryRunOverride)
        {
            return new PluginConfiguration
            {
                TargetLanguage = source.TargetLanguage,
                SecondaryLanguage = source.SecondaryLanguage,
                PreferBilingual = source.PreferBilingual,
                FormatOrder = source.FormatOrder,
                AutomationEnabled = source.AutomationEnabled,
                AutomationDryRun = dryRunOverride ?? source.AutomationDryRun,
                AutomationLibraryIds = source.AutomationLibraryIds == null
                    ? new List<string>()
                    : source.AutomationLibraryIds.ToList(),
                AutomationMaxItemsPerRun = source.AutomationMaxItemsPerRun,
                AutomationMaxCandidateFetchesPerItem = source.AutomationMaxCandidateFetchesPerItem,
                LibraryOverrides = source.LibraryOverrides == null
                    ? new List<LibraryPreferenceOverride>()
                    : source.LibraryOverrides.ToList()
            };
        }
    }

    internal static class M3AutomationStatusResponseExtensions
    {
        public static M3AutomationStatusResponse WithLibraryIds(this M3AutomationStatusResponse response, IEnumerable<string> ids)
        {
            if (ids != null)
            {
                response.AutomationLibraryIds.AddRange(ids.Where(id => !string.IsNullOrWhiteSpace(id)));
            }

            return response;
        }
    }
}
