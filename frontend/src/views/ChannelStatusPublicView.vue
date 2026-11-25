<template>
  <div :class="embedded ? '' : 'min-h-screen bg-gray-50 dark:bg-dark-950'">
    <!-- 渠道公告（单条覆盖式）：置顶通栏、吸顶悬浮、可关闭；管理员发布新公告后重新出现。
         管理端/用户面板嵌在 AppLayout 的 main 内边距里（p-4 md:p-6 lg:p-8），
         用等值负外边距抵消以铺满内容列；匿名页是根节点的直接子元素，天然通栏。 -->
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="-translate-y-2 opacity-0"
    >
      <div
        v-if="announcementVisible"
        class="sticky top-0 z-30 border-b border-amber-200 bg-amber-50 backdrop-blur-sm dark:border-amber-500/25 dark:bg-amber-500/10"
        :class="inLayout ? '-mx-4 -mt-4 md:-mx-6 md:-mt-6 lg:-mx-8 lg:-mt-8' : ''"
        data-testid="status-announcement"
      >
        <div class="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:px-6">
          <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-amber-100 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400">
            <Icon name="megaphone" size="sm" />
          </span>
          <div class="min-w-0 flex-1">
            <div class="text-[11px] font-semibold uppercase tracking-widest text-amber-700 dark:text-amber-400">
              {{ t('statusPage.annBadge') }} · {{ formatAnnouncementTime }}
            </div>
            <div class="mt-0.5 whitespace-pre-wrap break-words text-sm text-gray-800 dark:text-gray-100">{{ announcement?.content }}</div>
          </div>
          <div class="flex shrink-0 items-center gap-2 sm:pt-1">
            <button
              v-if="canEditAnnouncement"
              type="button"
              class="inline-flex items-center gap-1 rounded-lg border border-amber-300 bg-white/70 px-2.5 py-1 text-xs font-medium text-amber-800 transition-colors hover:bg-white dark:border-amber-500/40 dark:bg-dark-800/70 dark:text-amber-200 dark:hover:bg-dark-700"
              data-testid="status-announcement-edit"
              @click="announcementDialogOpen = true"
            >
              <Icon name="edit" size="sm" />
              {{ t('statusPage.annEdit') }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-lg border border-amber-300 bg-white px-2.5 py-1 text-xs font-semibold text-amber-900 shadow-sm transition-colors hover:bg-amber-100 dark:border-amber-500/40 dark:bg-dark-800 dark:text-amber-100 dark:hover:bg-dark-700"
              data-testid="status-announcement-close"
              @click="dismissAnnouncement"
            >
              <Icon name="x" size="sm" />
              {{ t('common.close') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>

    <main :class="embedded ? '' : 'mx-auto max-w-5xl px-4 py-8 sm:px-6'">
      <!-- 标题 -->
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('statusPage.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('statusPage.subtitle') }}
        </p>
      </div>

      <!-- 加载中 -->
      <div v-if="loading" class="flex justify-center py-20">
        <LoadingSpinner />
      </div>

      <!-- 功能未启用（后端 404） -->
      <EmptyState
        v-else-if="disabled"
        :title="t('statusPage.disabledTitle')"
        :description="t('statusPage.disabledDesc')"
      />

      <!-- 加载失败 -->
      <div v-else-if="loadFailed" class="py-16 text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('statusPage.loadFailed') }}</p>
        <button
          type="button"
          class="mt-4 inline-flex items-center rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-700"
          @click="load"
        >
          {{ t('statusPage.retry') }}
        </button>
      </div>

      <template v-else-if="snapshot">
        <!-- 整体状态横幅 -->
        <section
          class="mb-5 flex flex-col gap-4 rounded-2xl border p-5 transition-colors sm:flex-row sm:items-center sm:justify-between"
          :class="overallCardClass"
          data-testid="status-overall"
        >
          <div class="flex items-center gap-3">
            <span class="relative flex h-3 w-3 flex-shrink-0">
              <span
                v-if="snapshot.overall !== 'unknown'"
                class="absolute inline-flex h-full w-full rounded-full opacity-30"
                :class="STATUS_DOT[snapshot.overall]"
              ></span>
              <span
                class="relative inline-flex h-3 w-3 rounded-full"
                :class="STATUS_DOT[snapshot.overall]"
              ></span>
            </span>
            <div>
              <div class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ overallLabel }}
              </div>
              <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('statusPage.updatedAt', { time: updatedLabel }) }}
                · {{ t('statusPage.windowHours', { n: snapshot.window_hours }) }}
                · {{ t('statusPage.refreshEvery', { n: snapshot.refresh_seconds }) }}
              </div>
              <div
                v-if="embedded && statsScope === 'window'"
                class="mt-1 flex flex-wrap items-center gap-x-2 text-xs text-gray-500 dark:text-gray-400"
              >
                <span>{{ t('statusPage.avgGenCol') }} <span class="font-medium tabular-nums text-gray-700 dark:text-gray-200">{{ formatGenMs(snapshot.avg_gen_ms) }}</span></span>
                <span class="text-gray-300 dark:text-gray-600">·</span>
                <span>{{ t('statusPage.fastestCol') }} <span class="font-medium tabular-nums text-emerald-600 dark:text-emerald-400">{{ formatGenMs(snapshot.fastest_ms) }}</span></span>
                <span class="text-gray-300 dark:text-gray-600">·</span>
                <span>P95 <span class="font-medium tabular-nums text-gray-700 dark:text-gray-200">{{ formatGenMs(snapshot.p95_ms) }}</span></span>
                <span class="text-gray-300 dark:text-gray-600">·</span>
                <span>P99 <span class="font-medium tabular-nums text-gray-700 dark:text-gray-200">{{ formatGenMs(snapshot.p99_ms) }}</span></span>
                <span class="text-gray-300 dark:text-gray-600">·</span>
                <span>{{ t('statusPage.slowestCol') }} <span class="font-medium tabular-nums text-red-500 dark:text-red-400">{{ formatGenMs(snapshot.slowest_ms) }}</span></span>
              </div>
            </div>
          </div>
          <div class="flex flex-shrink-0 items-center gap-3">
            <button
              v-if="canEditAnnouncement"
              type="button"
              class="relative rounded-lg p-2 transition-colors"
              :class="announcement?.content
                ? 'bg-amber-50 text-amber-600 hover:bg-amber-100 dark:bg-amber-500/10 dark:text-amber-400 dark:hover:bg-amber-500/20'
                : 'bg-gray-100 text-gray-400 hover:bg-gray-200 hover:text-gray-600 dark:bg-dark-900 dark:text-gray-500 dark:hover:bg-dark-700'"
              :title="t('statusPage.annDialogTitle')"
              data-testid="status-announcement-btn"
              @click.stop="announcementDialogOpen = true"
            >
              <Icon name="megaphone" size="md" />
              <span
                v-if="announcement?.content"
                class="absolute -right-0.5 -top-0.5 flex h-2.5 w-2.5"
              >
                <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75"></span>
                <span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500"></span>
              </span>
            </button>
            <div class="flex gap-1 rounded-lg bg-gray-100 p-0.5 dark:bg-dark-900" data-testid="status-scope-toggle">
              <button
                v-for="opt in scopeOptions"
                :key="opt.value"
                type="button"
                class="rounded px-2.5 py-1 text-xs font-medium transition"
                :class="statsScope === opt.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
                @click="statsScope = opt.value"
              >{{ opt.label }}</button>
            </div>
            <AutoRefreshButton
              :enabled="autoRefresh.enabled.value"
            :interval-seconds="autoRefresh.intervalSeconds.value"
            :countdown="autoRefresh.countdown.value"
            :intervals="autoRefresh.intervals"
              @update:enabled="autoRefresh.setEnabled"
              @update:interval="autoRefresh.setInterval"
            />
          </div>
        </section>

        <ChannelAnnouncementDialog v-model:open="announcementDialogOpen" @saved="onAnnouncementSaved" />

        <!-- 首轮聚合未完成 -->
        <EmptyState
          v-if="snapshot.groups.length === 0"
          :title="t('statusPage.preparing')"
        />

        <template v-else>
          <!-- 状态筛选 + 搜索 -->
          <div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div
              class="inline-flex flex-wrap items-center gap-1 rounded-xl border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-800"
              data-testid="status-filter-tabs"
            >
              <button
                v-for="tab in filterTabs"
                :key="tab.value"
                type="button"
                class="rounded-lg px-3 py-1.5 text-xs font-medium transition-all duration-200"
                :class="filter === tab.value
                  ? 'bg-primary-600 text-white shadow-sm'
                  : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                :data-testid="`status-filter-${tab.value}`"
                @click="filter = tab.value"
              >
                {{ tab.label }}
              </button>
            </div>
            <div class="relative w-full sm:w-64">
              <svg
                class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <input
                v-model="search"
                type="text"
                :placeholder="t('statusPage.searchPlaceholder')"
                class="input h-9 pl-9 text-sm"
                data-testid="status-search"
              />
            </div>
          </div>

          <!-- 无匹配 -->
          <EmptyState
            v-if="filteredGroups.length === 0"
            :title="t('statusPage.noMatch')"
          />

          <!-- 分组列表 -->
          <div class="space-y-4" data-testid="status-group-list">
            <article
              v-for="group in filteredGroups"
              :key="group.id"
              class="group cursor-pointer rounded-2xl border bg-white p-5 shadow-sm transition-all duration-300 hover:-translate-y-0.5 hover:shadow-lg dark:bg-dark-800"
              :class="hoverBorderClass(group.status)"
              :data-testid="`status-group-${group.id}`"
              @click="toggleModels(group.id)"
            >
              <!-- 头部：名称/平台 + 状态 + 成功率 -->
              <div class="flex items-start justify-between gap-3">
                <div class="flex min-w-0 items-center gap-2.5">
                  <span class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl bg-gray-100 ring-1 ring-black/5 transition-colors group-hover:bg-gray-200/70 dark:bg-dark-700 dark:ring-white/10 dark:group-hover:bg-dark-600">
                    <PlatformIcon :platform="group.platform as GroupPlatform" size="sm" />
                  </span>
                  <div class="min-w-0">
                    <div class="truncate text-base font-semibold text-gray-900 dark:text-gray-100">
                      {{ group.name }}
                    </div>
                    <div class="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-gray-500 dark:text-gray-400">
                      <span>
                        {{ t('statusPage.avgResponseTime') }}
                        <span class="font-medium tabular-nums text-gray-700 dark:text-gray-200">{{ formatGenMs(scopeAvgMs(group)) }}</span>
                        <span class="ml-1 text-[10px] text-gray-400 dark:text-gray-500">{{ statsScope === '1h' ? t('statusPage.scopeLastHour') : t('statusPage.scopeFullWindow') }}</span>
                      </span>
                      <template v-if="embedded && statsScope === 'window'">
                        <span class="text-gray-300 dark:text-gray-600">·</span>
                        <span>{{ t('statusPage.fastestCol') }} <span class="font-medium tabular-nums text-emerald-600 dark:text-emerald-400">{{ formatGenMs(group.fastest_ms) }}</span></span>
                        <span class="text-gray-300 dark:text-gray-600">·</span>
                        <span>P95 <span class="font-medium tabular-nums text-gray-700 dark:text-gray-200">{{ formatGenMs(group.p95_ms) }}</span></span>
                        <span class="text-gray-300 dark:text-gray-600">·</span>
                        <span>P99 <span class="font-medium tabular-nums text-gray-700 dark:text-gray-200">{{ formatGenMs(group.p99_ms) }}</span></span>
                        <span class="text-gray-300 dark:text-gray-600">·</span>
                        <span>{{ t('statusPage.slowestCol') }} <span class="font-medium tabular-nums text-red-500 dark:text-red-400">{{ formatGenMs(group.slowest_ms) }}</span></span>
                      </template>
                    </div>
                  </div>
                </div>
                <div class="flex flex-shrink-0 flex-col items-end gap-1">
                  <span
                    class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold transition-colors"
                    :class="STATUS_CHIP[scopeStatus(group)]"
                  >
                    <span class="h-1.5 w-1.5 rounded-full" :class="STATUS_DOT[scopeStatus(group)]"></span>
                    {{ statusLabel(scopeStatus(group)) }}
                  </span>
                  <span
                    class="text-lg font-bold tabular-nums"
                    :class="rateTextClass(scopeRate(group))"
                    :title="statsScope === '1h' ? t('statusPage.scopeLastHour') : t('statusPage.scopeFullWindow')"
                  >
                    {{ formatRate(scopeRate(group)) }}
                  </span>
                </div>
              </div>

              <!-- 时间线（最旧在左，最新在右；悬浮显示气泡） -->
              <div class="mt-4 pt-3 border-t border-gray-100 dark:border-dark-700/60">
                <div class="mb-2 flex justify-between text-[10px] font-semibold uppercase tracking-widest text-gray-400">
                  <span>{{ t('statusPage.timeline', { n: snapshot.window_hours }) }}</span>
                  <span>{{ t('statusPage.past') }} → {{ t('statusPage.now') }}</span>
                </div>
                <StatusTimeline
                  :buckets="orderedTimeline(group)"
                  :selected-index="selectedBuckets[group.id] ?? null"
                  @select="(idx: number) => toggleBucket(group.id, idx)"
                />
              </div>

              <!-- 模型明细（可展开，带过渡）；点击时间线桶可下钻该时段数据。
                   管理端与登录用户面板可见（含模型成功率下钻）；匿名页只看分组级状态。 -->
              <div v-if="showModelDetail && (namedModels(group).length > 0 || isBucketSelected(group))" class="mt-3">
                <button
                  type="button"
                  class="inline-flex items-center gap-1 text-xs font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                  :data-testid="`status-models-toggle-${group.id}`"
                  @click.stop="toggleModels(group.id)"
                >
                  <svg
                    class="h-3.5 w-3.5 transition-transform duration-200"
                    :class="expanded[group.id] ? 'rotate-90' : ''"
                    fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                  </svg>
                  {{ t('statusPage.models') }}
                </button>
                <button
                  v-if="embedded"
                  type="button"
                  class="ml-3 inline-flex items-center gap-1 text-xs font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                  :data-testid="`status-accounts-toggle-${group.id}`"
                  @click.stop="toggleAccounts(group.id)"
                >
                  <svg
                    class="h-3.5 w-3.5 transition-transform duration-200"
                    :class="accountsExpanded[group.id] ? 'rotate-90' : ''"
                    fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                  </svg>
                  {{ t('statusPage.accountsToggle') }}
                </button>
                <Transition
                  enter-active-class="transition-all duration-300 ease-out"
                  enter-from-class="max-h-0 opacity-0"
                  enter-to-class="max-h-96 opacity-100"
                  leave-active-class="transition-all duration-200 ease-in"
                  leave-from-class="max-h-96 opacity-100"
                  leave-to-class="max-h-0 opacity-0"
                >
                  <div v-if="expanded[group.id]" class="mt-1.5 overflow-hidden">
                    <!-- 下钻提示条：显示当前选中时段 + 返回整窗入口 -->
                    <div
                      v-if="isBucketSelected(group)"
                      class="mb-2 flex items-center justify-between gap-2 rounded-md bg-gray-50 px-2.5 py-1.5 text-[11px] text-gray-600 dark:bg-dark-700/60 dark:text-gray-300"
                      data-testid="status-bucket-scope"
                    >
                      <span class="truncate">
                        {{ t('statusPage.drilldownTitle', { range: bucketRangeLabel(group, selectedBuckets[group.id] as number) }) }}
                      </span>
                      <button
                        type="button"
                        class="shrink-0 font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                        @click.stop="clearBucket(group.id)"
                      >
                        {{ t('statusPage.backToWindow') }}
                      </button>
                    </div>
                    <div v-else class="mb-2 text-[11px] text-gray-400 dark:text-gray-500">
                      {{ t('statusPage.drilldownHint') }}
                    </div>
                    <div class="overflow-x-auto">
                    <table class="w-full text-left text-xs">
                      <thead>
                        <tr class="text-[10px] uppercase tracking-widest text-gray-400">
                          <th class="py-1.5 pr-4 font-semibold">{{ t('statusPage.modelCol') }}</th>
                          <th class="py-1.5 pr-4 font-semibold">{{ t('statusPage.successRateCol') }}</th>
                          <th class="py-1.5 pr-3 font-semibold">{{ t('statusPage.avgGenCol') }}</th>
                          <th v-if="embedded" class="py-1.5 pr-3 font-semibold">{{ t('statusPage.fastestCol') }}</th>
                          <th v-if="embedded" class="py-1.5 pr-3 font-semibold">P95</th>
                          <th v-if="embedded" class="py-1.5 pr-3 font-semibold">P99</th>
                          <th v-if="embedded" class="py-1.5 font-semibold">{{ t('statusPage.slowestCol') }}</th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                        <tr
                          v-for="m in displayModels(group)"
                          :key="m.model"
                          class="transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
                        >
                          <td class="py-1.5 pr-4 font-mono text-gray-800 dark:text-gray-200">{{ m.model || t('statusPage.unknownModel') }}</td>
                          <td
                            class="py-1.5 pr-4 tabular-nums font-medium"
                            :class="rateTextClass(m.success_rate)"
                          >{{ formatRate(m.success_rate) }}</td>
                          <td class="py-1.5 pr-3 tabular-nums text-gray-600 dark:text-gray-300">{{ formatGenMs(m.avg_gen_ms) }}</td>
                          <td v-if="embedded" class="py-1.5 pr-3 tabular-nums text-emerald-600 dark:text-emerald-400">{{ formatGenMs(m.fastest_ms) }}</td>
                          <td v-if="embedded" class="py-1.5 pr-3 tabular-nums text-gray-600 dark:text-gray-300">{{ formatGenMs(m.p95_ms) }}</td>
                          <td v-if="embedded" class="py-1.5 pr-3 tabular-nums text-gray-600 dark:text-gray-300">{{ formatGenMs(m.p99_ms) }}</td>
                          <td v-if="embedded" class="py-1.5 tabular-nums text-red-500 dark:text-red-400">{{ formatGenMs(m.slowest_ms) }}</td>
                        </tr>
                        <tr v-if="displayModels(group).length === 0">
                          <td colspan="7" class="py-2 text-center text-gray-400">{{ t('statusPage.drilldownEmpty') }}</td>
                        </tr>
                      </tbody>
                    </table>
                    </div>
                  </div>
                </Transition>
              </div>

              <!-- 账号明细（管理端嵌入模式）：分组内各账号成功率进度条 + 逐小时 mini 时间线 -->
              <div v-if="embedded" class="mt-3">
                <Transition
                  enter-active-class="transition-all duration-300 ease-out"
                  enter-from-class="max-h-0 opacity-0"
                  enter-to-class="max-h-[2000px] opacity-100"
                  leave-active-class="transition-all duration-200 ease-in"
                  leave-from-class="max-h-[2000px] opacity-100"
                  leave-to-class="max-h-0 opacity-0"
                >
                  <div v-if="accountsExpanded[group.id]" class="overflow-hidden">
                    <div class="mb-2 flex flex-wrap items-center gap-2">
                      <div class="flex gap-1 rounded-lg bg-gray-100 p-0.5 dark:bg-dark-900">
                        <button
                          v-for="opt in accountRangeOptions"
                          :key="opt.value"
                          type="button"
                          class="rounded px-2 py-0.5 text-[10px] font-medium transition"
                          :class="accountRange[group.id] === opt.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400'"
                          @click.stop="setAccountRange(group.id, opt.value)"
                        >{{ opt.label }}</button>
                      </div>
                      <span v-if="isBucketSelected(group)" class="text-[10px] text-primary-600 dark:text-primary-400">
                        {{ t('statusPage.drilldownTitle', { range: bucketRangeLabel(group, selectedBuckets[group.id] as number) }) }}
                      </span>
                    </div>
                    <div v-if="accountStatsLoading[group.id]" class="py-3 text-center text-xs text-gray-400">
                      {{ t('common.loading') }}
                    </div>
                    <div v-else-if="(accountStats[group.id] || []).length === 0" class="py-3 text-center text-xs text-gray-400">
                      {{ t('statusPage.accountsEmpty') }}
                    </div>
                    <div v-else class="overflow-x-auto">
                      <table class="w-full text-left text-xs">
                        <thead>
                          <tr class="text-[10px] uppercase tracking-widest text-gray-400">
                            <th class="py-1.5 pr-4 font-semibold">{{ t('statusPage.accountCol') }}</th>
                            <th class="py-1.5 pr-4 text-right font-semibold">{{ t('statusPage.requestsCol') }}</th>
                            <th class="py-1.5 pr-4 text-right font-semibold">{{ t('statusPage.failuresCol') }}</th>
                            <th class="py-1.5 pr-4 text-right font-semibold">{{ t('statusPage.switchedCol') }}</th>
                            <th class="py-1.5 pr-4 text-right font-semibold">{{ t('statusPage.avgGenCol') }}</th>
                            <th class="py-1.5 pr-3 text-right font-semibold">{{ t('statusPage.fastestCol') }}</th>
                            <th class="py-1.5 pr-3 text-right font-semibold">P95</th>
                            <th class="py-1.5 pr-3 text-right font-semibold">P99</th>
                            <th class="py-1.5 pr-4 text-right font-semibold">{{ t('statusPage.slowestCol') }}</th>
                            <th class="py-1.5 pr-6 font-semibold">{{ t('statusPage.successRateCol') }}</th>
                            <th class="py-1.5 font-semibold">{{ t('statusPage.hourlyCol') }}</th>
                            <th class="py-1.5 font-semibold">{{ t('statusPage.errorsCol') }}</th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                          <tr
                            v-for="acc in accountStats[group.id] || []"
                            :key="acc.account_id"
                            class="transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
                            :data-testid="`status-account-${acc.account_id}`"
                          >
                            <td class="py-2 pr-4">
                              <span class="font-medium text-gray-800 dark:text-gray-200">{{ accountName(acc.account_id) }}</span>
                              <span class="ml-1.5 text-[10px] text-gray-400">#{{ acc.account_id }}</span>
                            </td>
                            <td class="py-2 pr-4 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ acc.requests }}</td>
                            <td class="py-2 pr-4 text-right tabular-nums" :class="acc.failures > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-400'">{{ acc.failures }}</td>
                            <td class="py-2 pr-4 text-right tabular-nums" :class="(acc.switched_to_requests || 0) > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-gray-400'" :title="t('statusPage.switchedHint')">{{ acc.switched_to_requests || '—' }}</td>
                            <td class="py-2 pr-4 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ formatGenMs(acc.avg_gen_ms) }}</td>
                            <td class="py-2 pr-3 text-right tabular-nums text-emerald-600 dark:text-emerald-400">{{ formatGenMs(acc.fastest_ms) }}</td>
                            <td class="py-2 pr-3 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ formatGenMs(acc.p95_ms) }}</td>
                            <td class="py-2 pr-3 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ formatGenMs(acc.p99_ms) }}</td>
                            <td class="py-2 pr-4 text-right tabular-nums text-red-500 dark:text-red-400">{{ formatGenMs(acc.slowest_ms) }}</td>
                            <td class="py-2 pr-6">
                              <div class="flex items-center gap-2">
                                <span class="w-12 shrink-0 tabular-nums font-medium" :class="rateTextClass(acc.success_rate)">{{ formatRate(acc.success_rate) }}</span>
                                <div class="h-1.5 w-24 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                                  <div
                                    class="h-full rounded-full transition-all duration-300"
                                    :class="accountBarClass(acc.success_rate)"
                                    :style="{ width: acc.success_rate >= 0 ? Math.min(100, acc.success_rate) + '%' : '0%' }"
                                  ></div>
                                </div>
                              </div>
                            </td>
                            <td class="py-2">
                              <div class="flex items-center gap-[3px]">
                                <div
                                  v-for="(h, hi) in accountHourlyBuckets(acc)"
                                  :key="hi"
                                  class="h-4 w-1.5 rounded-sm transition-transform hover:scale-y-125"
                                  :class="accountHourlyClass(h)"
                                  :title="accountHourlyTitle(h)"
                                ></div>
                              </div>
                            </td>
                            <td class="py-2">
                              <div class="flex max-w-[240px] flex-wrap items-center gap-1">
                                <span
                                  v-for="ec in acc.errors_by_code || []"
                                  :key="ec.status"
                                  class="rounded px-1.5 py-0.5 font-mono text-[10px] font-medium"
                                  :class="ec.status.startsWith('↻') ? 'bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300' : 'bg-red-50 text-red-600 dark:bg-red-900/30 dark:text-red-300'"
                                  :title="ec.status.startsWith('↻') ? (t('statusPage.recoveredHint') + ' — ' + (ec.sample_message || ec.status)) : (ec.sample_message || ec.status)"
                                >{{ ec.status }} ×{{ ec.count }}</span>
                                <span v-if="!(acc.errors_by_code || []).length" class="text-[10px] text-gray-400">—</span>
                              </div>
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </Transition>
              </div>
            </article>
          </div>
        </template>
      </template>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import StatusTimeline from '@/components/channelStatus/StatusTimeline.vue'
import { accountsAPI } from '@/api/admin/accounts'
import { getChannelStatusAccountStats, type AccountSuccessStat } from '@/api/admin/settings'
import ChannelAnnouncementDialog from '@/components/channelStatus/ChannelAnnouncementDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { getChannelStatusSnapshot } from '@/api/admin/settings'
import { getChannelStatus, getChannelStatusAnnouncement, type ChannelStatusAnnouncement, type ChannelHealthStatus, type ChannelStatusBucket, type ChannelStatusGroup, type ChannelStatusModelStat, type ChannelStatusSnapshot } from '@/api/channelStatus'
import { useAutoRefresh } from '@/composables/useAutoRefresh'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { formatDateTimeToMinute } from '@/utils/format'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  /** 管理端嵌入模式：跳过广场导航栏与页面级布局，由外层 AppLayout 提供。
   * 嵌入模式走管理端鉴权的完整快照接口（含耗时分布等详细指标）。 */
  embedded?: boolean
  /** 登录用户面板模式（API 后台侧边栏入口）：可展开模型成功率下钻，
   * 但不显示账号明细、耗时分布与公告编辑；走公开脱敏接口。 */
  panel?: boolean
}>(), {
  embedded: false,
  panel: false,
})
const embedded = computed(() => props.embedded)
// 模型成功率下钻：管理端与登录用户面板均可见（匿名公开页不展示）。
const showModelDetail = computed(() => props.embedded || props.panel)
// 嵌在 AppLayout 内容区内的模式（公告通栏需抵消 main 内边距才能铺满）。
const inLayout = computed(() => props.embedded || props.panel)

// 公告编辑入口：管理员在任何状态页变体（匿名页/用户面板/管理端嵌入）都可见
// （路由守卫会从 localStorage 恢复登录态，匿名页上 isAdmin 同样有效；PUT 需管理权限）。
const authStore = useAuthStore()
const canEditAnnouncement = computed(() => props.embedded || authStore.isAdmin)
const appStore = useAppStore()

const snapshot = ref<ChannelStatusSnapshot | null>(null)
const loading = ref(true)

// ---- 渠道公告（单条覆盖式）：置顶通栏可关闭；新公告（updated_at_ms 变化）会重新出现 ----
const ANNOUNCEMENT_DISMISS_KEY = 'sub2api:channel-status-ann-dismissed'
const announcement = ref<ChannelStatusAnnouncement | null>(null)
const announcementDialogOpen = ref(false)
// 已关闭公告的时间戳：localStorage 不是响应式来源，必须镜像成 ref，
// 否则关闭后可见性 computed 不会重新求值（点击关闭无反应）。
const announcementDismissedAtMs = ref<number | null>(readDismissedAnnouncementAtMs())

function readDismissedAnnouncementAtMs(): number | null {
  const raw = localStorage.getItem(ANNOUNCEMENT_DISMISS_KEY)
  if (raw === null) return null
  const parsed = Number(raw)
  return Number.isFinite(parsed) ? parsed : null
}

const announcementVisible = computed(
  () =>
    !!announcement.value?.content &&
    announcementDismissedAtMs.value !== announcement.value.updated_at_ms,
)

const formatAnnouncementTime = computed(() => {
  const ms = announcement.value?.updated_at_ms
  if (!ms) return ''
  return new Date(ms).toLocaleString()
})

function dismissAnnouncement(): void {
  if (!announcement.value) return
  announcementDismissedAtMs.value = announcement.value.updated_at_ms
  localStorage.setItem(ANNOUNCEMENT_DISMISS_KEY, String(announcement.value.updated_at_ms))
}

function onAnnouncementSaved(content: string, updatedAtMs: number): void {
  announcement.value = content ? { content, updated_at_ms: updatedAtMs } : null
  if (!content) {
    announcementDismissedAtMs.value = null
    localStorage.removeItem(ANNOUNCEMENT_DISMISS_KEY)
  }
}

// 统计口径：默认「最近 1 小时」滚动窗口（所有用户可见），可切「全部窗口」。
// 详细分布（最快/最长/P95/P99）与账号明细仅管理端（embedded）可见。
type StatsScope = '1h' | 'window'
const statsScope = ref<StatsScope>('1h')
const scopeOptions = computed(() => [
  { value: '1h' as StatsScope, label: t('statusPage.scopeLastHour') },
  { value: 'window' as StatsScope, label: t('statusPage.scopeFullWindow') },
])

function scopeRate(group: ChannelStatusGroup): number {
  if (statsScope.value === '1h' && group.last_hour) return group.last_hour.success_rate
  return group.success_rate
}

function scopeAvgMs(group: ChannelStatusGroup): number {
  if (statsScope.value === '1h' && group.last_hour) return group.last_hour.avg_gen_ms
  return group.avg_gen_ms
}

/** 状态徽章跟随当前口径：1h 模式按 last_hour 成功率用后端下发的阈值重新分级，
 * 全窗口模式沿用后端算好的 group.status（同一套阈值）。 */
function scopeStatus(group: ChannelStatusGroup): ChannelHealthStatus {
  if (statsScope.value !== '1h' || !group.last_hour) return group.status
  const th = snapshot.value?.thresholds
  if (!th) return group.status
  if (group.last_hour.requests + group.last_hour.failures <= 0) return 'unknown'
  const rate = group.last_hour.success_rate
  if (rate >= th.healthy) return 'operational'
  if (rate >= th.degraded) return 'degraded'
  return 'down'
}

/** 最近 1 小时对应的原始（未截空桶）时间线末桶。 */
function lastRawBucket(group: ChannelStatusGroup): ChannelStatusBucket | null {
  const tl = group.timeline || []
  return tl.length ? tl[tl.length - 1] : null
}
const disabled = ref(false)
const loadFailed = ref(false)
const expanded = ref<Record<number, boolean>>({})
const filter = ref<'all' | ChannelHealthStatus>('all')
const search = ref('')

// 状态 → 展示样式（点/胶囊/条块）。配色 + transition，无自写动画。
const STATUS_DOT: Record<ChannelHealthStatus, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  down: 'bg-red-500',
  unknown: 'bg-gray-300 dark:bg-dark-600'
}

const STATUS_CHIP: Record<ChannelHealthStatus, string> = {
  operational: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400',
  degraded: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-400',
  down: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400',
  unknown: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}

// hover 时的边框强调色：跟随分组状态。
function hoverBorderClass(status: ChannelHealthStatus): string {
  if (status === 'down') return 'border-gray-200 hover:border-red-300 dark:border-dark-700 dark:hover:border-red-500/40'
  if (status === 'degraded') return 'border-gray-200 hover:border-amber-300 dark:border-dark-700 dark:hover:border-amber-500/40'
  if (status === 'operational') return 'border-gray-200 hover:border-emerald-300 dark:border-dark-700 dark:hover:border-emerald-500/40'
  return 'border-gray-200 hover:border-gray-300 dark:border-dark-700 dark:hover:border-dark-600'
}

// 成功率数字着色：正常绿 / 降级黄 / 故障红 / 无数据灰。
function rateTextClass(rate: number): string {
  if (rate < 0) return 'text-gray-400'
  if (rate >= 99) return 'text-emerald-600 dark:text-emerald-400'
  if (rate >= 90) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

// 故障优先展示：down > degraded > operational > unknown，同档按成功率升序。
const STATUS_SEVERITY: Record<ChannelHealthStatus, number> = {
  down: 0,
  degraded: 1,
  operational: 2,
  unknown: 3
}

const filterTabs = computed(() => [
  { value: 'all' as const, label: t('statusPage.tabAll') },
  { value: 'operational' as const, label: statusLabel('operational') },
  { value: 'degraded' as const, label: statusLabel('degraded') },
  { value: 'down' as const, label: statusLabel('down') },
  { value: 'unknown' as const, label: statusLabel('unknown') }
])

const sortedGroups = computed<ChannelStatusGroup[]>(() => {
  const list = [...(snapshot.value?.groups ?? [])]
  list.sort((a, b) => {
    const sev = STATUS_SEVERITY[a.status] - STATUS_SEVERITY[b.status]
    if (sev !== 0) return sev
    return a.success_rate - b.success_rate
  })
  return list
})

const filteredGroups = computed<ChannelStatusGroup[]>(() => {
  const keyword = search.value.trim().toLowerCase()
  return sortedGroups.value.filter((g) => {
    if (filter.value !== 'all' && g.status !== filter.value) return false
    if (keyword && !g.name.toLowerCase().includes(keyword)) return false
    return true
  })
})

const overallCardClass = computed(() => {
  const overall = snapshot.value?.overall
  if (overall === 'operational') return 'border-emerald-200 bg-emerald-50/60 dark:border-emerald-500/20 dark:bg-emerald-500/5'
  if (overall === 'degraded') return 'border-amber-200 bg-amber-50/60 dark:border-amber-500/20 dark:bg-amber-500/5'
  if (overall === 'down') return 'border-red-200 bg-red-50/60 dark:border-red-500/20 dark:bg-red-500/5'
  return 'border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800'
})

const overallLabel = computed(() => {
  const key = `statusPage.overall${cap(snapshot.value?.overall ?? 'unknown')}`
  return t(key)
})

const updatedLabel = computed(() => {
  const ms = snapshot.value?.generated_at_ms
  if (!ms) return '—'
  return formatDateTimeToMinute(new Date(ms))
})

function statusLabel(status: ChannelHealthStatus): string {
  return t(`statusPage.status${cap(status)}`)
}

function cap(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}

/** success_rate 为 -1 表示窗口内无请求。 */
function formatRate(rate: number): string {
  if (rate < 0) return '—'
  return `${Math.round(rate * 100) / 100}%`
}

function formatGenMs(ms: number): string {
  if (!ms || ms <= 0) return '—'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

/** 模型明细只展示有成功记录的有名条目:空名行(早期阶段失败无模型归属)与
 * 从未成功过的模型(客户端误配模型名,全部失败)不显示,避免噪音行。 */
function namedModels(group: ChannelStatusGroup): ChannelStatusModelStat[] {
  return (group.models || []).filter((m) => m.model !== '' && m.requests > 0)
}

/** API 返回的桶为最旧在前，直接左→右渲染即为「过去 → 现在」。
 * 无请求的空桶直接截掉（用户偏好：时间线只看有流量的时段，避免大片空白）；
 * 全窗口都无流量时回退为原始 24 桶，避免时间线整个消失。
 * 选中桶索引、下钻明细与区间文案均基于本函数的返回值，截断后仍全局一致。 */
function orderedTimeline(group: ChannelStatusGroup): ChannelStatusBucket[] {
  const all = group.timeline ?? []
  const active = all.filter((b) => b.requests + b.failures > 0)
  return active.length > 0 ? active : all
}

/** 单桶时间宽度（毫秒）：窗口 24 桶均分。截掉空桶后相邻桶不再连续，
 * 区间计算不能用相邻桶差值，必须用固定桶宽。 */
function bucketWidthMs(): number {
  return ((snapshot.value?.window_hours || 24) * 3600_000) / 24
}

function toggleModels(id: number): void {
  expanded.value = { ...expanded.value, [id]: !expanded.value[id] }
}

// ---- 账号明细（管理端嵌入模式）：分组内各账号成功率 + 逐小时 mini 进度条 ----
const accountsExpanded = ref<Record<number, boolean>>({})
const accountStats = ref<Record<number, AccountSuccessStat[]>>({})
const accountStatsLoading = ref<Record<number, boolean>>({})
const accountNameById = ref<Record<number, string>>({})
let accountNamesLoaded = false

function toggleAccounts(id: number): void {
  accountsExpanded.value = { ...accountsExpanded.value, [id]: !accountsExpanded.value[id] }
  if (accountsExpanded.value[id]) void loadAccountStats(id)
}

async function loadAccountStats(groupId: number): Promise<void> {
  if (accountStats.value[groupId]) return
  if (!snapshot.value) return
  accountStatsLoading.value = { ...accountStatsLoading.value, [groupId]: true }
  try {
    if (!accountNamesLoaded) {
      accountNamesLoaded = true
      try {
        const res: any = await accountsAPI.list(1, 500, {})
        const items: any[] = res?.items || []
        const byId: Record<number, string> = {}
        for (const a of items) byId[a.id] = a.name || `#${a.id}`
        accountNameById.value = byId
      } catch {
        accountNameById.value = {}
      }
    }
    const res = await getChannelStatusAccountStats(groupId, 168, accountStatsRangeFor(groupId))
    accountStats.value = { ...accountStats.value, [groupId]: res.accounts || [] }
  } catch {
    accountStats.value = { ...accountStats.value, [groupId]: [] }
  } finally {
    accountStatsLoading.value = { ...accountStatsLoading.value, [groupId]: false }
  }
}

function accountName(id: number): string {
  return accountNameById.value[id] || `#${id}`
}

function accountBarClass(rate: number): string {
  if (rate < 0) return 'bg-gray-300 dark:bg-dark-600'
  if (rate >= 99) return 'bg-emerald-500'
  if (rate >= 90) return 'bg-amber-500'
  return 'bg-red-500'
}

interface AccountHourlyCell {
  requests: number
  failures: number
  success_rate: number
  status: string
  label: string
}

function accountHourlyBuckets(acc: AccountSuccessStat): AccountHourlyCell[] {
  const hourly = acc.hourly || []
  return hourly.map((h) => ({
    requests: h.requests,
    failures: h.failures,
    success_rate: h.success_rate,
    status: h.status,
    label: new Date(h.hour_start_ms).toLocaleString(),
  }))
}

function accountHourlyClass(h: AccountHourlyCell): string {
  if (h.requests === 0) return 'bg-gray-200 dark:bg-dark-700'
  switch (h.status) {
    case 'operational':
      return 'bg-emerald-500/80'
    case 'degraded':
      return 'bg-amber-500/80'
    case 'down':
      return 'bg-red-500/80'
    default:
      return 'bg-gray-300 dark:bg-dark-600'
  }
}

function accountHourlyTitle(h: AccountHourlyCell): string {
  if (h.requests === 0) return `${h.label} | ${t('statusPage.noRequests')}`
  return `${h.label} | ${t('statusPage.requestsCol')}: ${h.requests} | ${t('statusPage.failuresCol')}: ${h.failures} | ${t('statusPage.successRateCol')}: ${h.success_rate.toFixed(1)}%`
}

// ---- 时间线下钻：点击某桶 → 模型明细切换为该时段数据；再点同一桶返回整窗 ----
const selectedBuckets = ref<Record<number, number | null>>({})

function isBucketSelected(group: ChannelStatusGroup): boolean {
  return (selectedBuckets.value[group.id] ?? null) !== null
}

function toggleBucket(groupId: number, idx: number): void {
  const current = selectedBuckets.value[groupId] ?? null
  const next = current === idx ? null : idx
  selectedBuckets.value = { ...selectedBuckets.value, [groupId]: next }
  // 下钻时自动展开明细，用户不用再点一次展开按钮。
  if (next !== null) expanded.value = { ...expanded.value, [groupId]: true }
}

function clearBucket(groupId: number): void {
  selectedBuckets.value = { ...selectedBuckets.value, [groupId]: null }
}

// 账号明细统计范围：window=快照整窗；3d/7d=自定义区间；选中时间线桶时优先按该小时联动。
const accountRange = ref<Record<number, string>>({})
const accountRangeOptions = computed(() => [
  { value: 'window', label: t('statusPage.rangeWindow') },
  { value: '3d', label: t('statusPage.range3d') },
  { value: '7d', label: t('statusPage.range7d') },
])

function setAccountRange(groupId: number, value: string): void {
  accountRange.value = { ...accountRange.value, [groupId]: value }
  delete accountStats.value[groupId]
  void loadAccountStats(groupId)
}

// 桶联动：选中时间线某小时后，重新加载账号统计为该小时区间。
watch(selectedBuckets, (sel, prev) => {
  for (const key of Object.keys(sel)) {
    const groupId = Number(key)
    if (!accountsExpanded.value[groupId]) continue
    const now = (sel as Record<number, number | null>)[groupId]
    const before = ((prev || {}) as Record<number, number | null>)[groupId]
    if (now !== before) {
      delete accountStats.value[groupId]
      void loadAccountStats(groupId)
    }
  }
})

function accountStatsRangeFor(groupId: number): { start?: string; end?: string } {
  // 选中时间线桶时优先联动该小时
  const bucketIdx = selectedBuckets.value[groupId]
  if (bucketIdx !== null && bucketIdx !== undefined) {
    const group = (snapshot.value?.groups || []).find((g) => g.id === groupId)
    const buckets = group ? orderedTimeline(group) : []
    const bucket = buckets[bucketIdx]
    if (bucket) {
      const endMs = bucket.bucket_start_ms + bucketWidthMs()
      return { start: new Date(bucket.bucket_start_ms).toISOString(), end: new Date(endMs).toISOString() }
    }
  }
  const range = accountRange.value[groupId] || 'window'
  if (range === '3d' || range === '7d') {
    const hours = range === '3d' ? 72 : 168
    return { start: new Date(Date.now() - hours * 3600_000).toISOString() }
  }
  return {}
}


/** 模型明细数据源：选中时间线桶时展示该桶的按小时统计，否则展示整窗统计。
 * 下钻视图保留「该时段只有失败」的模型行（requests=0 也有诊断价值），只滤掉空名；
 * 整窗视图维持原有过滤。 */
function displayModels(group: ChannelStatusGroup): ChannelStatusModelStat[] {
  const idx = selectedBuckets.value[group.id] ?? null
  if (idx !== null) {
    const bucket = orderedTimeline(group)[idx]
    return (bucket?.models ?? []).filter((m) => m.model !== '')
  }
  // 默认视图跟随口径：最近 1 小时 → 原始时间线末桶（与 last_hour 同窗口）；
  // 全窗口 → 整窗模型统计。
  if (statsScope.value === '1h') {
    const bucket = lastRawBucket(group)
    return (bucket?.models ?? []).filter((m) => m.model !== '')
  }
  return namedModels(group)
}

/** 桶时间范围文案：起点取桶的 bucket_start_ms，终点按固定桶宽推算
 * （空桶被截掉后相邻桶不连续，不能用相邻差值）。 */
function bucketRangeLabel(group: ChannelStatusGroup, idx: number): string {
  const timeline = orderedTimeline(group)
  const bucket = timeline[idx]
  if (!bucket) return ''
  const start = new Date(bucket.bucket_start_ms)
  const end = new Date(bucket.bucket_start_ms + bucketWidthMs())
  return `${formatDateTimeToMinute(start)} ~ ${formatDateTimeToMinute(end)}`
}

async function load(): Promise<void> {
  try {
    snapshot.value = embedded.value
      ? await getChannelStatusSnapshot()
      : await getChannelStatus()
    try {
      announcement.value = await getChannelStatusAnnouncement()
    } catch {
      announcement.value = null
    }
    disabled.value = false
    loadFailed.value = false
  } catch (err) {
    const status = (err as { status?: number })?.status
    if (status === 404) {
      disabled.value = true
      snapshot.value = null
    } else {
      loadFailed.value = true
    }
  } finally {
    loading.value = false
  }
}

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-page-autorefresh',
  intervals: [30, 60, 120, 300],
  defaultInterval: 60,
  defaultEnabled: true,
  onRefresh: load,
  shouldPause: () => document.hidden
})

onMounted(() => {
  // 独立形态导航条需要站点名/Logo。
  void appStore.fetchPublicSettings()
  void load()
  // defaultEnabled/本地记忆为开启时需显式启动定时器（组合式函数不会自启）。
  autoRefresh.setEnabled(autoRefresh.enabled.value)
})
</script>
