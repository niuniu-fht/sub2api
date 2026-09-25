<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6">
      <!-- 页头 -->
      <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.title') }}</h1>
            <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.description') }}</p>
          </div>
          <button
            type="button"
            class="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-60 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
            :disabled="loading || savingRouting || savingPricing || savingThresholds"
            @click="loadAll"
          >
            {{ t('admin.imageBilling.refresh') }}
          </button>
        </div>
      </div>

      <!-- 主体:左分组 / 右配置 -->
      <div class="grid gap-6 lg:grid-cols-[300px_1fr] lg:items-start">
        <!-- 分组列表 -->
        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-wrap gap-2 rounded-xl bg-gray-100 p-1 dark:bg-dark-900">
            <button
              v-for="tab in platformTabs"
              :key="tab.key"
              type="button"
              class="flex-1 rounded-lg px-3 py-2 text-sm font-medium transition"
              :class="activePlatform === tab.key ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-200' : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="switchPlatform(tab.key)"
            >
              {{ tab.key === 'openai' ? t('admin.imageBilling.groups.platformOpenAI') : t('admin.imageBilling.groups.platformGemini') }}
            </button>
          </div>

          <div class="mt-3">
            <input
              v-model="groupSearch"
              type="text"
              class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              :placeholder="t('admin.imageBilling.groups.searchPlaceholder')"
            />
          </div>

          <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.imageBilling.groups.showing', { shown: filteredGroups.length, total: platformGroups.length }) }}
          </div>

          <div class="mt-3 max-h-[60vh] space-y-2 overflow-y-auto pr-1">
            <button
              v-for="group in filteredGroups"
              :key="group.id"
              type="button"
              class="w-full rounded-xl border p-3 text-left transition"
              :class="group.id === selectedGroupId
                ? 'border-primary-400 bg-primary-50 shadow-sm dark:border-primary-500/60 dark:bg-primary-900/20'
                : 'border-gray-200 bg-white hover:border-primary-200 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900/60 dark:hover:border-primary-700 dark:hover:bg-dark-900'"
              @click="selectedGroupId = group.id"
            >
              <div class="flex items-center justify-between gap-2">
                <span class="min-w-0 truncate text-sm font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
                <span class="shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium uppercase text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                  {{ String(group.platform || '').toUpperCase() }}
                </span>
              </div>
              <div class="mt-1 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                <span>#{{ group.id }}</span>
                <span>{{ t('admin.imageBilling.groups.availableAccounts', { count: accountOptionsForGroup(group.id).length }) }}</span>
              </div>
              <div class="mt-1.5 flex items-center gap-1.5 text-[11px] text-gray-500 dark:text-gray-400">
                <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-dark-700">{{ routingRuleCount(group.id) }}</span>
                <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-dark-700">{{ pricingCellCount(group.id) }}</span>
              </div>
            </button>
            <div
              v-if="!loading && filteredGroups.length === 0"
              class="rounded-xl border border-dashed border-gray-200 px-3 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
            >
              {{ t('admin.imageBilling.groups.noMatch') }}
            </div>
          </div>
        </div>

        <!-- 右侧配置面板 -->
        <div v-if="selectedGroup" class="space-y-6">
          <div class="flex flex-wrap gap-2 rounded-xl border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <button
              v-for="tab in configTabs"
              :key="tab"
              type="button"
              class="rounded-lg px-4 py-2 text-sm font-medium transition"
              :class="activeTab === tab ? 'bg-primary-600 text-white shadow-sm' : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="activeTab = tab"
            >
              {{ tab === 'routing' ? t('admin.imageBilling.routing.tab') : t('admin.imageBilling.pricing.tab') }}
            </button>
          </div>

          <!-- 路由规则 -->
          <div v-if="activeTab === 'routing'" class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.routing.title') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.subtitle') }}</p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <button
                  type="button"
                  class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-60"
                  :disabled="editorOpen"
                  @click="openRuleEditor()"
                >
                  + {{ t('admin.imageBilling.routing.addRule') }}
                </button>
                <button
                  type="button"
                  class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-60"
                  :disabled="savingRouting || loading"
                  @click="saveRouting"
                >
                  {{ savingRouting ? t('admin.imageBilling.pricing.saving') : t('admin.imageBilling.routing.save') }}
                </button>
              </div>
            </div>

            <!-- 规则表 -->
            <div class="mt-5 overflow-x-auto">
              <table v-if="displayRules.length > 0" class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead>
                  <tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    <th class="px-3 py-2">{{ t('admin.imageBilling.routing.condition') }}</th>
                    <th v-if="activePlatform === 'openai'" class="px-3 py-2">{{ t('admin.imageBilling.routing.mode') }}</th>
                    <th class="px-3 py-2">{{ t('admin.imageBilling.routing.accounts') }}</th>
                    <th class="px-3 py-2 text-right">{{ t('admin.imageBilling.routing.actions') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                  <tr v-for="(rule, index) in displayRules" :key="ruleKey(rule, index)" class="align-top">
                    <td class="px-3 py-3">
                      <div class="flex flex-wrap items-center gap-1.5">
                        <template v-if="activePlatform === 'openai'">
                          <span
                            v-for="quality in rule.qualities"
                            :key="quality"
                            class="rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-semibold text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-200"
                          >
                            {{ qualityLabel(quality) }}
                          </span>
                          <span class="text-gray-400">×</span>
                        </template>
                        <span
                          v-for="tier in rule.tiers"
                          :key="tier"
                          class="rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-semibold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200"
                        >
                          {{ tier }}
                        </span>
                        <template v-if="activePlatform === 'gemini'">
                          <span class="text-gray-400">×</span>
                          <span
                            v-for="ratio in (rule as GeminiRule).aspect_ratios"
                            :key="ratio"
                            class="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-200"
                          >
                            {{ aspectRatioLabel(ratio) }}
                          </span>
                        </template>
                      </div>
                    </td>
                    <td v-if="activePlatform === 'openai'" class="px-3 py-3">
                      <span
                        class="rounded-full px-2 py-0.5 text-xs font-medium"
                        :class="(rule as OpenAIRule).mode === 'round_robin'
                          ? 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-200'
                          : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'"
                      >
                        {{ (rule as OpenAIRule).mode === 'round_robin' ? t('admin.imageBilling.routing.editor.modeRoundRobin') : t('admin.imageBilling.routing.editor.modePriority') }}
                      </span>
                    </td>
                    <td class="px-3 py-3">
                      <div class="flex flex-wrap items-center gap-1.5">
                        <span
                          v-for="(accountId, accountIndex) in rule.account_ids.slice(0, 6)"
                          :key="accountId"
                          class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-200"
                        >
                          <span class="font-semibold text-primary-600 dark:text-primary-300">{{ accountIndex + 1 }}</span>
                          {{ accountName(accountId) }}
                        </span>
                        <span v-if="rule.account_ids.length > 6" class="text-xs text-gray-400">+{{ rule.account_ids.length - 6 }}</span>
                      </div>
                    </td>
                    <td class="px-3 py-3 text-right">
                      <button type="button" class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300" @click="openRuleEditor(index)">
                        {{ t('admin.imageBilling.routing.edit') }}
                      </button>
                      <span class="mx-1.5 text-gray-300 dark:text-dark-600">|</span>
                      <button type="button" class="text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-300" @click="confirmDeleteRule(index)">
                        {{ t('admin.imageBilling.routing.delete') }}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
              <div
                v-else
                class="rounded-xl border border-dashed border-primary-200 bg-primary-50/40 px-4 py-8 text-center dark:border-primary-900/60 dark:bg-primary-900/10"
              >
                <div class="text-sm font-medium text-primary-700 dark:text-primary-200">{{ t('admin.imageBilling.routing.empty') }}</div>
                <div class="mt-1 text-xs text-primary-700/70 dark:text-primary-200/70">{{ t('admin.imageBilling.routing.emptyHint') }}</div>
              </div>
            </div>

            <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.qualityRuleFirst') }}</p>

            <!-- 档位兜底(仅 OpenAI) -->
            <div v-if="activePlatform === 'openai'" class="mt-5 border-t border-gray-200 pt-4 dark:border-dark-700">
              <div>
                <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.routing.fallbackTitle') }}</div>
                <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.fallbackHint') }}</div>
                <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.fallbackModeHint') }}</div>
              </div>
              <div class="mt-3 space-y-3">
                <div
                  v-for="tier in billingTiers"
                  :key="tier"
                  class="rounded-xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/60"
                >
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="w-10 shrink-0 rounded-md bg-emerald-600 px-2 py-1 text-center text-xs font-bold text-white">{{ tier }}</span>
                    <select
                      :value="fallbackModeFor(selectedGroupId, tier)"
                      class="shrink-0 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs font-medium text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                      :title="fallbackModeFor(selectedGroupId, tier) === 'round_robin' ? t('admin.imageBilling.routing.roundRobinHint') : t('admin.imageBilling.routing.priorityHint')"
                      @change="setFallbackMode(selectedGroupId, tier, ($event.target as HTMLSelectElement).value)"
                    >
                      <option value="priority">{{ t('admin.imageBilling.routing.priorityMode') }}</option>
                      <option value="round_robin">{{ t('admin.imageBilling.routing.roundRobinMode') }}</option>
                    </select>
                    <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
                      <template v-if="fallbackAccountIDs(selectedGroupId, tier).length > 0">
                        <span
                          v-for="(accountId, accountIndex) in fallbackAccountIDs(selectedGroupId, tier)"
                          :key="accountId"
                          class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2 py-1 text-xs text-gray-700 shadow-sm dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
                        >
                          <span class="font-semibold text-primary-600 dark:text-primary-300">{{ accountIndex + 1 }}</span>
                          {{ accountName(accountId) }}
                          <button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" :disabled="accountIndex === 0" @click="moveFallbackAccount(tier, accountIndex, -1)">↑</button>
                          <button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" :disabled="accountIndex === fallbackAccountIDs(selectedGroupId, tier).length - 1" @click="moveFallbackAccount(tier, accountIndex, 1)">↓</button>
                          <button type="button" class="text-red-400 hover:text-red-600" @click="removeFallbackAccount(tier, accountId)">×</button>
                        </span>
                      </template>
                      <span v-else class="text-xs text-gray-400">{{ t('admin.imageBilling.routing.fallbackEmpty') }}</span>
                    </div>
                    <select
                      class="shrink-0 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                      value=""
                      @change="addFallbackAccount(tier, ($event.target as HTMLSelectElement).value); ($event.target as HTMLSelectElement).value = ''"
                    >
                      <option value="" disabled>+ {{ t('admin.imageBilling.routing.addAccount') }}</option>
                      <option v-for="account in availableFallbackAccounts(tier)" :key="account.id" :value="account.id">
                        {{ accountLabel(account) }}
                      </option>
                    </select>
                  </div>
                </div>
              </div>
            </div>

            <p class="mt-4 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.note') }}</p>
          </div>

          <!-- 计费价格 -->
          <div v-else class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.pricing.title') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.pricing.subtitle') }}</p>
              </div>
              <button
                type="button"
                class="shrink-0 rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-60"
                :disabled="savingPricing || loading"
                @click="savePricing"
              >
                {{ savingPricing ? t('admin.imageBilling.pricing.saving') : t('admin.imageBilling.pricing.save') }}
              </button>
            </div>

            <!-- Quality × 档位矩阵 -->
            <div class="mt-5">
              <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.pricing.qualityMatrix') }}</div>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.pricing.qualityMatrixHint') }}</p>
              <div class="mt-3 overflow-x-auto">
                <table class="min-w-full text-sm">
                  <thead>
                    <tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                      <th class="px-3 py-2">quality</th>
                      <th v-for="tier in billingTiers" :key="tier" class="px-3 py-2">{{ tier }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                    <tr v-for="quality in imageQualities" :key="quality">
                      <td class="px-3 py-2">
                        <span class="rounded-full bg-indigo-50 px-2.5 py-1 text-xs font-semibold text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-200">
                          {{ qualityLabel(quality) }}
                        </span>
                      </td>
                      <td v-for="tier in billingTiers" :key="tier" class="px-3 py-2">
                        <input
                          v-model="pricingDraft.quality_prices[quality][tier]"
                          type="number"
                          min="0"
                          step="0.001"
                          class="w-32 rounded-lg border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                          :placeholder="qualityPricePlaceholder(quality, tier)"
                        />
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- 档位单价 -->
            <div class="mt-6">
              <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.pricing.flatPrices') }}</div>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.pricing.flatHint') }}</p>
              <div class="mt-3 grid gap-4 sm:grid-cols-3">
                <label v-for="tier in billingTiers" :key="tier" class="block">
                  <span class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ tier }} · {{ t('admin.imageBilling.pricing.unit') }}</span>
                  <input
                    v-model="pricingDraft.flat_prices[tier]"
                    type="number"
                    min="0"
                    step="0.001"
                    class="mt-1.5 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                    :placeholder="flatPricePlaceholder(tier)"
                  />
                </label>
              </div>
            </div>

            <!-- 倍率 -->
            <div class="mt-6 rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/60">
              <div class="flex items-center gap-3">
                <Toggle v-model="pricingDraft.image_rate_independent" />
                <div>
                  <div class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.imageBilling.pricing.rateIndependent') }}</div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.pricing.rateIndependentHint') }}</div>
                </div>
                <input
                  v-model="pricingDraft.image_rate_multiplier"
                  type="number"
                  min="0"
                  step="0.1"
                  class="ml-auto w-28 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                />
              </div>
            </div>

            <p class="mt-4 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.pricing.openAIOnly') }}</p>
          </div>
        </div>

        <!-- 未选择分组 -->
        <div v-else class="flex min-h-[320px] items-center justify-center rounded-2xl border border-dashed border-gray-200 bg-gray-50 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800/60 dark:text-gray-400">
          {{ loading ? t('admin.imageBilling.loading') : t('admin.imageBilling.groups.noMatch') }}
        </div>
      </div>

      <!-- 阈值 -->
      <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.thresholds.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.thresholds.description') }}</p>
        </div>
        <div class="mt-4 grid gap-5 md:grid-cols-2">
          <label class="block">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.thresholds.twoK') }}</span>
            <input
              v-model.number="thresholdForm.two_k_pixel_threshold"
              type="number"
              min="1"
              step="1"
              class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              placeholder="3000000"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.thresholds.twoKHint') }}</p>
          </label>
          <label class="block">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.thresholds.fourK') }}</span>
            <input
              v-model.number="thresholdForm.four_k_pixel_threshold"
              type="number"
              min="1"
              step="1"
              class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              placeholder="6000000"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.thresholds.fourKHint') }}</p>
          </label>
        </div>
        <div class="mt-5 flex flex-wrap gap-3">
          <button
            type="button"
            class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-60"
            :disabled="savingThresholds || loading"
            @click="saveThresholds"
          >
            {{ t('admin.imageBilling.thresholds.save') }}
          </button>
          <button
            type="button"
            class="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
            @click="resetThresholdDefaults"
          >
            {{ t('admin.imageBilling.thresholds.resetDefaults') }}
          </button>
        </div>
      </div>

      <!-- 快速验证 -->
      <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.imageBilling.validator.title') }}</h2>
        <div class="mt-4 grid gap-4 md:grid-cols-3">
          <label class="block">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.validator.width') }}</span>
            <input v-model.number="sample.width" type="number" min="1" step="1" class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white" />
          </label>
          <label class="block">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.validator.height') }}</span>
            <input v-model.number="sample.height" type="number" min="1" step="1" class="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white" />
          </label>
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
            <div class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.validator.area', { area: formatNumber(sampleArea) }) }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.imageBilling.validator.sample', { w: sample.width, h: sample.height, tier: sampleTier }) }}
            </div>
          </div>
        </div>

        <div class="mt-5 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-900">
              <tr>
                <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">{{ t('admin.imageBilling.validator.tierColumn') }}</th>
                <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">{{ t('admin.imageBilling.validator.ruleColumn') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
              <tr>
                <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">1K</td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ t('admin.imageBilling.validator.rule1K', { twoK: formatNumber(thresholdForm.two_k_pixel_threshold) }) }}</td>
              </tr>
              <tr>
                <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">2K</td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ t('admin.imageBilling.validator.rule2K', { twoK: formatNumber(thresholdForm.two_k_pixel_threshold), fourK: formatNumber(thresholdForm.four_k_pixel_threshold) }) }}</td>
              </tr>
              <tr>
                <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">4K</td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ t('admin.imageBilling.validator.rule4K', { fourK: formatNumber(thresholdForm.four_k_pixel_threshold) }) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- 规则编辑弹窗 -->
    <BaseDialog :show="editorOpen" :title="editorTitle" width="wide" @close="closeRuleEditor">
      <div class="space-y-5">
        <!-- quality(仅 OpenAI) -->
        <div v-if="editorPlatform === 'openai'">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.routing.editor.quality') }}</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <button
              v-for="quality in imageQualities"
              :key="quality"
              type="button"
              class="rounded-full border px-3.5 py-1.5 text-sm font-semibold transition"
              :class="editorDraft.qualities.includes(quality)
                ? 'border-indigo-500 bg-indigo-50 text-indigo-700 shadow-sm dark:border-indigo-400 dark:bg-indigo-900/30 dark:text-indigo-100'
                : 'border-gray-200 bg-white text-gray-600 hover:border-indigo-300 hover:text-indigo-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300 dark:hover:border-indigo-700'"
              @click="toggleEditorQuality(quality)"
            >
              {{ qualityLabel(quality) }}
            </button>
          </div>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.editor.qualityHint') }}</p>
        </div>

        <!-- 档位 -->
        <div>
          <div class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.routing.editor.tier') }}</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <button
              v-for="tier in billingTiers"
              :key="tier"
              type="button"
              class="rounded-full border px-3.5 py-1.5 text-sm font-semibold transition"
              :class="editorDraft.tiers.includes(tier)
                ? 'border-emerald-500 bg-emerald-50 text-emerald-700 shadow-sm dark:border-emerald-400 dark:bg-emerald-900/30 dark:text-emerald-100'
                : 'border-gray-200 bg-white text-gray-600 hover:border-emerald-300 hover:text-emerald-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300 dark:hover:border-emerald-700'"
              @click="toggleEditorTier(tier)"
            >
              {{ tier }}
            </button>
          </div>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.editor.tierHint') }}</p>
        </div>

        <!-- 比例(仅 Gemini) -->
        <div v-if="editorPlatform === 'gemini'">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.groups.platformGemini') }}</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <button
              v-for="ratio in geminiAspectRatioOptions"
              :key="ratio.value"
              type="button"
              class="rounded-full border px-3.5 py-1.5 text-sm font-semibold transition"
              :class="editorDraft.aspect_ratios.includes(ratio.value)
                ? 'border-amber-500 bg-amber-50 text-amber-700 shadow-sm dark:border-amber-400 dark:bg-amber-900/30 dark:text-amber-100'
                : 'border-gray-200 bg-white text-gray-600 hover:border-amber-300 hover:text-amber-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300 dark:hover:border-amber-700'"
              @click="toggleEditorAspectRatio(ratio.value)"
            >
              {{ ratio.label }}
            </button>
          </div>
        </div>

        <!-- 调度方式(仅 OpenAI) -->
        <div v-if="editorPlatform === 'openai'">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.routing.editor.mode') }}</div>
          <div class="mt-2 grid gap-2 sm:grid-cols-2">
            <button
              v-for="mode in routingModes"
              :key="mode"
              type="button"
              class="rounded-xl border px-3.5 py-2.5 text-left text-sm transition"
              :class="editorDraft.mode === mode
                ? 'border-primary-500 bg-primary-50 text-primary-700 shadow-sm dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-100'
                : 'border-gray-200 bg-white text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300'"
              @click="editorDraft.mode = mode"
                      >
                        <span class="block font-semibold">{{ mode === 'round_robin' ? t('admin.imageBilling.routing.editor.modeRoundRobin') : t('admin.imageBilling.routing.editor.modePriority') }}</span>
                        <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">{{ mode === 'round_robin' ? t('admin.imageBilling.routing.editor.modeRoundRobinHint') : t('admin.imageBilling.routing.editor.modePriorityHint') }}</span>
                      </button>
          </div>
        </div>

        <!-- 账号链 -->
        <div>
          <div class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.imageBilling.routing.editor.accounts') }}</div>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.imageBilling.routing.editor.accountsHint') }}</p>
          <div class="mt-3">
            <input
              v-model="editorSearch"
              type="text"
              class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              :placeholder="t('admin.imageBilling.routing.editor.searchPlaceholder')"
            />
          </div>
          <div class="mt-3 grid gap-3 md:grid-cols-2">
            <!-- 已选 -->
            <div class="rounded-xl border border-primary-100 bg-primary-50/50 p-3 dark:border-primary-900/40 dark:bg-primary-900/10">
              <div class="mb-2 text-xs font-semibold text-primary-700 dark:text-primary-200">{{ t('admin.imageBilling.routing.editor.selectedTitle') }}</div>
              <div class="max-h-60 space-y-1.5 overflow-y-auto pr-1">
                <div
                  v-for="(accountId, index) in editorDraft.account_ids"
                  :key="accountId"
                  class="rounded-lg bg-white px-2.5 py-2 text-xs shadow-sm dark:bg-dark-900"
                >
                  <div class="flex flex-wrap items-center gap-1.5">
                    <span class="shrink-0 rounded-full bg-primary-600 px-2 py-0.5 font-semibold text-white">{{ t('admin.imageBilling.routing.editor.priorityN', { n: index + 1 }) }}</span>
                    <span class="min-w-0 flex-1 truncate font-medium text-gray-900 dark:text-gray-100">{{ accountName(accountId) }}</span>
                    <button type="button" class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800" :disabled="index === 0" @click="moveEditorAccount(index, -1)">
                      {{ t('admin.imageBilling.routing.editor.moveUp') }}
                    </button>
                    <button type="button" class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800" :disabled="index === editorDraft.account_ids.length - 1" @click="moveEditorAccount(index, 1)">
                      {{ t('admin.imageBilling.routing.editor.moveDown') }}
                    </button>
                    <button type="button" class="rounded border border-red-200 px-1.5 py-0.5 text-red-600 hover:bg-red-50 dark:border-red-900/60 dark:text-red-300 dark:hover:bg-red-900/20" @click="removeEditorAccount(accountId)">
                      {{ t('admin.imageBilling.routing.editor.remove') }}
                    </button>
                  </div>
                </div>
                <div
                  v-if="editorDraft.account_ids.length === 0"
                  class="rounded-lg border border-dashed border-primary-200 px-3 py-4 text-center text-xs text-primary-700/70 dark:border-primary-900/60 dark:text-primary-200/70"
                >
                  {{ t('admin.imageBilling.routing.fallbackEmpty') }}
                </div>
              </div>
            </div>
            <!-- 可添加 -->
            <div class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
              <div class="mb-2 text-xs font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.imageBilling.routing.editor.availableTitle') }}</div>
              <div class="max-h-60 space-y-1 overflow-y-auto pr-1">
                <button
                  v-for="account in availableEditorAccounts"
                  :key="account.id"
                  type="button"
                  class="flex w-full items-start gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition hover:bg-gray-50 dark:hover:bg-dark-900"
                  @click="addEditorAccount(account.id)"
                >
                  <span class="mt-0.5 shrink-0 rounded border border-gray-300 px-1.5 py-0.5 text-[11px] text-gray-500 dark:border-dark-600 dark:text-gray-300">+</span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate font-medium text-gray-900 dark:text-white">{{ accountLabel(account) }}</span>
                    <span class="block truncate text-gray-500 dark:text-gray-400">
                      {{ String(account.platform || '').toUpperCase() }} · {{ t('admin.imageBilling.routing.editor.concurrency', { n: account.concurrency ?? '-' }) }}
                    </span>
                  </span>
                </button>
                <div
                  v-if="availableEditorAccounts.length === 0"
                  class="rounded-lg border border-dashed border-gray-200 px-3 py-4 text-center text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400"
                >
                  {{ t('admin.imageBilling.routing.editor.noAvailable') }}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button
            type="button"
            class="rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
            @click="closeRuleEditor"
          >
            {{ t('admin.imageBilling.routing.editor.cancel') }}
          </button>
          <button
            type="button"
            class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700"
            @click="confirmRuleEditor"
          >
            {{ t('admin.imageBilling.routing.editor.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- 删除确认 -->
    <ConfirmDialog
      :show="deleteConfirmShow"
      :title="t('admin.imageBilling.routing.delete')"
      :message="t('admin.imageBilling.routing.deleteConfirm')"
      danger
      @confirm="doDeleteRule"
      @cancel="deleteConfirmShow = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import { accountsAPI, groupsAPI } from '@/api/admin'
import {
  getGeminiImageBillingRoutingSettings,
  getImageBillingAccountRoutingSettings,
  getImageBillingThresholdSettings,
  updateGeminiImageBillingRoutingSettings,
  updateImageBillingAccountRoutingSettings,
  updateImageBillingThresholdSettings
} from '@/api/admin/settings'
import { useAppStore } from '@/stores'
import type { Account, AdminGroup, ImageQualityPrices, UpdateGroupRequest } from '@/types'
import { getImagePricePlaceholder, imageQualityPricePlaceholders } from './groupsImagePricing'

const { t } = useI18n()
const appStore = useAppStore()

const DEFAULT_2K_THRESHOLD = 3000000
const DEFAULT_4K_THRESHOLD = 6000000

type BillingTier = '1K' | '2K' | '4K'
type ImageQuality = 'low' | 'medium' | 'high' | 'xhigh' | 'max'
type RoutingMode = 'priority' | 'round_robin'

type OpenAIRule = {
  qualities: ImageQuality[]
  tiers: BillingTier[]
  mode: RoutingMode
  account_ids: number[]
}

type GeminiRule = {
  tiers: BillingTier[]
  aspect_ratios: string[]
  account_ids: number[]
}

const billingTiers: BillingTier[] = ['1K', '2K', '4K']
const imageQualities: ImageQuality[] = ['low', 'medium', 'high', 'xhigh', 'max']
const routingModes: RoutingMode[] = ['priority', 'round_robin']

const geminiAspectRatioOptions = [
  { value: '*', labelKey: 'all' },
  { value: '1:1', label: '1:1' },
  { value: '16:9', label: '16:9' },
  { value: '9:16', label: '9:16' },
  { value: '4:3', label: '4:3' },
  { value: '3:4', label: '3:4' },
]

const platformTabs: Array<{ key: 'openai' | 'gemini' }> = [{ key: 'openai' }, { key: 'gemini' }]
const configTabs: Array<'routing' | 'pricing'> = ['routing', 'pricing']

// ===================== 全局状态 =====================
const loading = ref(false)
const savingRouting = ref(false)
const savingPricing = ref(false)
const savingThresholds = ref(false)

const groups = ref<AdminGroup[]>([])
const accounts = ref<Account[]>([])
const activePlatform = ref<'openai' | 'gemini'>('openai')
const selectedGroupId = ref<number | null>(null)
const activeTab = ref<'routing' | 'pricing'>('routing')
const groupSearch = ref('')

const thresholdForm = reactive({
  two_k_pixel_threshold: DEFAULT_2K_THRESHOLD,
  four_k_pixel_threshold: DEFAULT_4K_THRESHOLD,
})

const sample = reactive({ width: 1536, height: 2048 })
const sampleArea = computed(() => Math.max(0, Number(sample.width) || 0) * Math.max(0, Number(sample.height) || 0))
const sampleTier = computed(() => {
  if (sampleArea.value > thresholdForm.four_k_pixel_threshold) return '4K'
  if (sampleArea.value > thresholdForm.two_k_pixel_threshold) return '2K'
  return '1K'
})

// ===================== 路由表单状态 =====================
const openAIRules = reactive<Record<string, OpenAIRule[]>>({})
const fallbackForm = reactive<Record<string, { one_k: number[]; two_k: number[]; four_k: number[] }>>({})
const fallbackModes = reactive<Record<string, Record<BillingTier, RoutingMode>>>({})
const geminiRules = reactive<Record<string, GeminiRule[]>>({})

// ===================== 价格表单状态 =====================
type PricingDraft = {
  flat_prices: Record<BillingTier, string>
  quality_prices: Record<ImageQuality, Record<BillingTier, string>>
  image_rate_independent: boolean
  image_rate_multiplier: string
}

const pricingDraft = reactive<PricingDraft>({
  flat_prices: { '1K': '', '2K': '', '4K': '' },
  quality_prices: {
    low: { '1K': '', '2K': '', '4K': '' },
    medium: { '1K': '', '2K': '', '4K': '' },
    high: { '1K': '', '2K': '', '4K': '' },
    xhigh: { '1K': '', '2K': '', '4K': '' },
    max: { '1K': '', '2K': '', '4K': '' },
  },
  image_rate_independent: false,
  image_rate_multiplier: '1',
})

// ===================== 编辑弹窗状态 =====================
const editorOpen = ref(false)
const editorPlatform = ref<'openai' | 'gemini'>('openai')
const editorIndex = ref<number | null>(null)
const editorSearch = ref('')
const editorDraft = reactive<{
  qualities: ImageQuality[]
  tiers: BillingTier[]
  aspect_ratios: string[]
  mode: RoutingMode
  account_ids: number[]
}>({
  qualities: [],
  tiers: [],
  aspect_ratios: [],
  mode: 'priority',
  account_ids: [],
})

const deleteConfirmShow = ref(false)
const deleteConfirmIndex = ref<number | null>(null)

// ===================== 计算属性 =====================
const platformGroups = computed(() =>
  groups.value
    .filter((group) => {
      const platform = String(group.platform || '').toLowerCase()
      return activePlatform.value === 'gemini'
        ? ['gemini', 'composite'].includes(platform)
        : ['openai', 'composite', 'grok'].includes(platform)
    })
    .sort((a, b) => a.id - b.id),
)

const filteredGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return platformGroups.value
  return platformGroups.value.filter((group) =>
    [group.name, group.platform, group.id].some((value) => String(value ?? '').toLowerCase().includes(keyword)),
  )
})

const selectedGroup = computed(() => platformGroups.value.find((group) => group.id === selectedGroupId.value) || null)

const editorTitle = computed(() =>
  editorIndex.value === null
    ? t('admin.imageBilling.routing.editor.newTitle')
    : t('admin.imageBilling.routing.editor.editTitle'),
)

const displayRules = computed<any[]>(() => {
  if (!selectedGroupId.value) return []
  const key = String(selectedGroupId.value)
  return activePlatform.value === 'openai' ? openAIRules[key] || [] : geminiRules[key] || []
})

const availableEditorAccounts = computed(() => {
  if (!selectedGroupId.value) return []
  const selected = new Set(editorDraft.account_ids)
  const keyword = editorSearch.value.trim().toLowerCase()
  return accountOptionsForGroup(selectedGroupId.value)
    .filter((account) => !selected.has(account.id))
    .filter((account) => {
      if (!keyword) return true
      return [account.id, account.name, account.platform].some((value) =>
        String(value ?? '').toLowerCase().includes(keyword),
      )
    })
})

// ===================== 工具函数 =====================
function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(Math.trunc(Number(value) || 0))
}

function normalizeAccountIDs(ids?: (number | string)[], legacyID?: number | string): number[] {
  const seen = new Set<number>()
  const result: number[] = []
  const add = (value: unknown) => {
    const id = Math.trunc(Number(value) || 0)
    if (id <= 0 || seen.has(id)) return
    seen.add(id)
    result.push(id)
  }
  ;(ids || []).forEach(add)
  add(legacyID)
  return result
}

function normalizeQuality(value?: string): ImageQuality | null {
  const quality = String(value || '').trim().toLowerCase()
  return (imageQualities as string[]).includes(quality) ? (quality as ImageQuality) : null
}

function normalizeTier(value?: string): BillingTier | null {
  const tier = String(value || '').trim().toUpperCase()
  return tier === '1K' || tier === '2K' || tier === '4K' ? tier : null
}

function normalizeGeminiRatio(value?: string): string {
  const ratio = String(value || '').trim().toLowerCase().replace(/\s+/g, '')
  if (!ratio || ratio === 'auto' || ratio === 'all' || ratio === 'any') return '*'
  return ratio
}

function accountBelongsToGroup(account: Account, groupId: number): boolean {
  if (Array.isArray(account.group_ids) && account.group_ids.includes(groupId)) return true
  return Number((account as any).group_id || 0) === groupId
}

function accountOptionsForGroupPlatform(groupId: number, platform: 'openai' | 'gemini'): Account[] {
  const allowedPlatforms = platform === 'gemini' ? ['gemini'] : ['openai', 'grok']
  return accounts.value
    .filter((account) => {
      const platform = String(account.platform || '').toLowerCase()
      if (!allowedPlatforms.includes(platform)) return false
      if (account.status !== 'active' || account.schedulable !== true) return false
      return accountBelongsToGroup(account, groupId)
    })
    .sort((a, b) => a.id - b.id)
}

function accountOptionsForGroup(groupId: number): Account[] {
  return accountOptionsForGroupPlatform(groupId, activePlatform.value)
}

function accountLabel(account: Account): string {
  return `#${account.id} ${account.name || '-'}`
}

function accountName(accountId: number): string {
  const account = accounts.value.find((item) => item.id === accountId)
  return account ? account.name || `#${account.id}` : `#${accountId}`
}

function qualityLabel(quality: ImageQuality): string {
  return t(`admin.imageBilling.quality.${quality}`)
}

function aspectRatioLabel(value: string): string {
  const ratio = normalizeGeminiRatio(value)
  return ratio === '*' ? t('admin.imageBilling.aspect.all') : ratio
}

function enabledAccountIDSetForGroupPlatform(groupId: number, platform: 'openai' | 'gemini'): Set<number> {
  return new Set(accountOptionsForGroupPlatform(groupId, platform).map((account) => account.id))
}

function routingRuleCount(groupId: number): string {
  const key = String(groupId)
  const rules = activePlatform.value === 'openai' ? openAIRules[key] : geminiRules[key]
  return t('admin.imageBilling.routing.ruleCount', { count: rules?.length || 0 })
}

function pricingCellCount(groupId: number): string {
  const group = groups.value.find((item) => item.id === groupId)
  if (!group) return t('admin.imageBilling.pricing.cellCount', { count: 0 })
  let count = 0
  for (const quality of imageQualities) {
    for (const tier of billingTiers) {
      if (group.image_quality_prices?.[quality]?.[tier] !== undefined) count += 1
    }
  }
  return t('admin.imageBilling.pricing.cellCount', { count })
}

function ruleKey(rule: OpenAIRule | GeminiRule, index: number): string {
  return `${index}:${rule.account_ids.join(',')}:${rule.tiers.join(',')}`
}

// ===================== 路由:加载与回显 =====================
type ImageBillingRoutingSettingsInput = {
  groups?: Record<string, {
    one_k_account_ids?: number[]
    two_k_account_ids?: number[]
    four_k_account_ids?: number[]
    one_k_account_id?: number
    two_k_account_id?: number
    four_k_account_id?: number
    tier_modes?: Record<string, string>
    rules?: Array<{ quality?: string; tier?: string; mode?: string; account_ids?: number[] }>
  }>
}

type GeminiRoutingSettingsInput = {
  groups?: Record<string, { rules?: Array<{ tier?: string; aspect_ratio?: string; account_ids?: number[] }> }>
}

function applyOpenAIRouting(settings: ImageBillingRoutingSettingsInput): void {
  Object.keys(openAIRules).forEach((key) => delete openAIRules[key])
  Object.keys(fallbackForm).forEach((key) => delete fallbackForm[key])
  Object.keys(fallbackModes).forEach((key) => delete fallbackModes[key])

  for (const [groupId, routing] of Object.entries(settings.groups || {})) {
    fallbackForm[groupId] = {
      one_k: normalizeAccountIDs(routing.one_k_account_ids, routing.one_k_account_id),
      two_k: normalizeAccountIDs(routing.two_k_account_ids, routing.two_k_account_id),
      four_k: normalizeAccountIDs(routing.four_k_account_ids, routing.four_k_account_id),
    }
    const tierModesRaw = routing.tier_modes || {}
    fallbackModes[groupId] = {
      '1K': tierModesRaw['1K'] === 'round_robin' ? 'round_robin' : 'priority',
      '2K': tierModesRaw['2K'] === 'round_robin' ? 'round_robin' : 'priority',
      '4K': tierModesRaw['4K'] === 'round_robin' ? 'round_robin' : 'priority',
    }
    // 后端每条规则只表示一个 quality×tier 组合;按「调度方式 + 账号链」反向合并回展示规则。
    const merged: OpenAIRule[] = []
    const index = new Map<string, OpenAIRule>()
    for (const rule of routing.rules || []) {
      const quality = normalizeQuality(rule.quality)
      const tier = normalizeTier(rule.tier)
      if (!quality || !tier) continue
      const mode: RoutingMode = rule.mode === 'round_robin' ? 'round_robin' : 'priority'
      const accountIDs = normalizeAccountIDs(rule.account_ids)
      if (accountIDs.length === 0) continue
      const key = `${mode}:${accountIDs.join(',')}`
      let target = index.get(key)
      if (!target) {
        target = { qualities: [], tiers: [], mode, account_ids: accountIDs }
        index.set(key, target)
        merged.push(target)
      }
      if (!target.qualities.includes(quality)) target.qualities.push(quality)
      if (!target.tiers.includes(tier)) target.tiers.push(tier)
    }
    if (merged.length > 0) openAIRules[groupId] = merged
  }
  pruneRoutingToEnabledAccounts()
}

function applyGeminiRouting(settings: GeminiRoutingSettingsInput): void {
  Object.keys(geminiRules).forEach((key) => delete geminiRules[key])

  for (const [groupId, routing] of Object.entries(settings.groups || {})) {
    const merged: GeminiRule[] = []
    const index = new Map<string, GeminiRule>()
    for (const rule of routing.rules || []) {
      const tier = normalizeTier(rule.tier)
      if (!tier) continue
      const ratio = normalizeGeminiRatio(rule.aspect_ratio)
      const accountIDs = normalizeAccountIDs(rule.account_ids)
      if (accountIDs.length === 0) continue
      const key = accountIDs.join(',')
      let target = index.get(key)
      if (!target) {
        target = { tiers: [], aspect_ratios: [], account_ids: accountIDs }
        index.set(key, target)
        merged.push(target)
      }
      if (!target.tiers.includes(tier)) target.tiers.push(tier)
      if (!target.aspect_ratios.includes(ratio)) target.aspect_ratios.push(ratio)
    }
    if (merged.length > 0) geminiRules[groupId] = merged
  }
  pruneRoutingToEnabledAccounts()
}

function pruneRoutingToEnabledAccounts(): void {
  for (const group of groups.value) {
    const key = String(group.id)
    // OpenAI 规则/兜底与 Gemini 规则分别按各自平台的账号集合剪枝,避免页签切换时误删。
    const enabled = enabledAccountIDSetForGroupPlatform(group.id, 'openai')
    const geminiEnabled = enabledAccountIDSetForGroupPlatform(group.id, 'gemini')
    const fallback = fallbackForm[key]
    if (fallback) {
      fallback.one_k = normalizeAccountIDs(fallback.one_k).filter((id) => enabled.has(id))
      fallback.two_k = normalizeAccountIDs(fallback.two_k).filter((id) => enabled.has(id))
      fallback.four_k = normalizeAccountIDs(fallback.four_k).filter((id) => enabled.has(id))
    }
    if (openAIRules[key]) {
      openAIRules[key] = openAIRules[key]
        .map((rule) => ({ ...rule, account_ids: normalizeAccountIDs(rule.account_ids).filter((id) => enabled.has(id)) }))
        .filter((rule) => rule.account_ids.length > 0)
      if (openAIRules[key].length === 0) delete openAIRules[key]
    }
    if (geminiRules[key]) {
      geminiRules[key] = geminiRules[key]
        .map((rule) => ({ ...rule, account_ids: normalizeAccountIDs(rule.account_ids).filter((id) => geminiEnabled.has(id)) }))
        .filter((rule) => rule.account_ids.length > 0)
      if (geminiRules[key].length === 0) delete geminiRules[key]
    }
  }
}

// ===================== 路由:保存 =====================
type ImageBillingRoutingPayload = {
  groups: Record<string, {
    one_k_account_ids: number[]
    two_k_account_ids: number[]
    four_k_account_ids: number[]
    tier_modes?: Record<string, string>
    rules: Array<{ quality: ImageQuality; tier: BillingTier; mode: RoutingMode; account_ids: number[] }>
  }>
}

type GeminiRoutingPayload = {
  groups: Record<string, { rules: Array<{ tier: BillingTier; aspect_ratio: string; account_ids: number[] }> }>
}

function buildOpenAIRoutingPayload(): ImageBillingRoutingPayload {
  const payload: ImageBillingRoutingPayload['groups'] = {}
  const groupIDs = new Set([...Object.keys(openAIRules), ...Object.keys(fallbackForm)])
  for (const rawID of groupIDs) {
    const groupId = Math.trunc(Number(rawID) || 0)
    if (groupId <= 0) continue
    const enabled = enabledAccountIDSetForGroupPlatform(groupId, 'openai')
    const fallback = fallbackForm[rawID]
    const oneK = normalizeAccountIDs(fallback?.one_k).filter((id) => enabled.has(id))
    const twoK = normalizeAccountIDs(fallback?.two_k).filter((id) => enabled.has(id))
    const fourK = normalizeAccountIDs(fallback?.four_k).filter((id) => enabled.has(id))

    // 展示规则 → quality × tier 组合展开(去重)
    const seen = new Set<string>()
    const rules: ImageBillingRoutingPayload['groups'][string]['rules'] = []
    for (const rule of openAIRules[rawID] || []) {
      const accountIDs = normalizeAccountIDs(rule.account_ids).filter((id) => enabled.has(id))
      if (accountIDs.length === 0) continue
      for (const quality of rule.qualities) {
        if (!normalizeQuality(quality)) continue
        for (const tier of rule.tiers) {
          if (!normalizeTier(tier)) continue
          const key = `${quality}:${tier}`
          if (seen.has(key)) continue
          seen.add(key)
          rules.push({ quality, tier, mode: rule.mode, account_ids: accountIDs })
        }
      }
    }

    if (oneK.length > 0 || twoK.length > 0 || fourK.length > 0 || rules.length > 0) {
      const tierModes: Record<string, string> = {}
      for (const tier of billingTiers) {
        if (fallbackModes[rawID]?.[tier] === 'round_robin') tierModes[tier] = 'round_robin'
      }
      payload[rawID] = { one_k_account_ids: oneK, two_k_account_ids: twoK, four_k_account_ids: fourK, tier_modes: tierModes, rules }
    }
  }
  return { groups: payload }
}

function buildGeminiRoutingPayload(): GeminiRoutingPayload {
  const payload: GeminiRoutingPayload['groups'] = {}
  for (const [groupId, rules] of Object.entries(geminiRules)) {
    const enabled = enabledAccountIDSetForGroupPlatform(Math.trunc(Number(groupId) || 0), 'gemini')
    const merged = new Map<string, GeminiRoutingPayload['groups'][string]['rules'][number]>()
    for (const rule of rules) {
      const accountIDs = normalizeAccountIDs(rule.account_ids).filter((id) => enabled.has(id))
      if (accountIDs.length === 0) continue
      for (const tier of rule.tiers) {
        if (!normalizeTier(tier)) continue
        for (const ratio of rule.aspect_ratios) {
          const key = `${tier}:${ratio}`
          const existing = merged.get(key)
          if (existing) {
            existing.account_ids = normalizeAccountIDs([...existing.account_ids, ...accountIDs])
          } else {
            merged.set(key, { tier, aspect_ratio: ratio, account_ids: accountIDs })
          }
        }
      }
    }
    const list = Array.from(merged.values())
    if (list.length > 0) payload[groupId] = { rules: list }
  }
  return { groups: payload }
}

async function saveRouting(): Promise<void> {
  savingRouting.value = true
  try {
    const [openAISettings, geminiSettings] = await Promise.all([
      updateImageBillingAccountRoutingSettings(buildOpenAIRoutingPayload()),
      updateGeminiImageBillingRoutingSettings(buildGeminiRoutingPayload()),
    ])
    applyOpenAIRouting(openAISettings as ImageBillingRoutingSettingsInput)
    applyGeminiRouting(geminiSettings as GeminiRoutingSettingsInput)
    appStore.showSuccess(t('admin.imageBilling.routing.saved'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.imageBilling.routing.saveFailed'))
  } finally {
    savingRouting.value = false
  }
}

// ===================== 兜底账号编辑 =====================
function fallbackModeFor(groupId: number | null, tier: BillingTier): RoutingMode {
  if (groupId === null) return 'priority'
  return fallbackModes[String(groupId)]?.[tier] === 'round_robin' ? 'round_robin' : 'priority'
}

function setFallbackMode(groupId: number | null, tier: BillingTier, mode: string): void {
  if (groupId === null) return
  if (!fallbackModes[String(groupId)]) {
    fallbackModes[String(groupId)] = { '1K': 'priority', '2K': 'priority', '4K': 'priority' }
  }
  fallbackModes[String(groupId)][tier] = mode === 'round_robin' ? 'round_robin' : 'priority'
}

function fallbackAccountIDs(groupId: number | null, tier: BillingTier): number[] {
  if (groupId === null) return []
  const fallback = fallbackForm[String(groupId)]
  if (!fallback) return []
  return tier === '1K' ? fallback.one_k : tier === '2K' ? fallback.two_k : fallback.four_k
}

function availableFallbackAccounts(tier: BillingTier): Account[] {
  if (!selectedGroupId.value) return []
  const selected = new Set(fallbackAccountIDs(selectedGroupId.value, tier))
  return accountOptionsForGroup(selectedGroupId.value).filter((account) => !selected.has(account.id))
}

function addFallbackAccount(tier: BillingTier, rawID: string): void {
  if (!selectedGroupId.value) return
  const key = String(selectedGroupId.value)
  if (!fallbackForm[key]) {
    fallbackForm[key] = { one_k: [], two_k: [], four_k: [] }
  }
  const id = Math.trunc(Number(rawID) || 0)
  if (id <= 0) return
  const list = tier === '1K' ? fallbackForm[key].one_k : tier === '2K' ? fallbackForm[key].two_k : fallbackForm[key].four_k
  if (!list.includes(id)) list.push(id)
}

function removeFallbackAccount(tier: BillingTier, accountId: number): void {
  if (!selectedGroupId.value) return
  const fallback = fallbackForm[String(selectedGroupId.value)]
  if (!fallback) return
  const list = tier === '1K' ? fallback.one_k : tier === '2K' ? fallback.two_k : fallback.four_k
  const index = list.indexOf(accountId)
  if (index !== -1) list.splice(index, 1)
}

function moveFallbackAccount(tier: BillingTier, index: number, direction: -1 | 1): void {
  if (!selectedGroupId.value) return
  const fallback = fallbackForm[String(selectedGroupId.value)]
  if (!fallback) return
  const list = tier === '1K' ? fallback.one_k : tier === '2K' ? fallback.two_k : fallback.four_k
  const target = index + direction
  if (index < 0 || target < 0 || index >= list.length || target >= list.length) return
  const [item] = list.splice(index, 1)
  list.splice(target, 0, item)
}

// ===================== 规则编辑弹窗 =====================
function openRuleEditor(ruleIndex?: number): void {
  if (!selectedGroupId.value) return
  editorPlatform.value = activePlatform.value
  editorIndex.value = ruleIndex ?? null
  editorSearch.value = ''
  const rules = displayRules.value
  const source = ruleIndex !== undefined ? rules[ruleIndex] : null
  if (source) {
    if (editorPlatform.value === 'openai') {
      const rule = source as OpenAIRule
      editorDraft.qualities = [...rule.qualities]
      editorDraft.mode = rule.mode
    } else {
      const rule = source as GeminiRule
      editorDraft.aspect_ratios = [...rule.aspect_ratios]
    }
    editorDraft.tiers = [...source.tiers]
    editorDraft.account_ids = [...source.account_ids]
  } else {
    editorDraft.qualities = ['high']
    editorDraft.tiers = ['1K']
    editorDraft.aspect_ratios = ['*']
    editorDraft.mode = 'priority'
    editorDraft.account_ids = []
  }
  editorOpen.value = true
}

function closeRuleEditor(): void {
  editorOpen.value = false
  editorIndex.value = null
}

function toggleEditorQuality(quality: ImageQuality): void {
  const index = editorDraft.qualities.indexOf(quality)
  if (index === -1) editorDraft.qualities.push(quality)
  else if (editorDraft.qualities.length > 1) editorDraft.qualities.splice(index, 1)
}

function toggleEditorTier(tier: BillingTier): void {
  const index = editorDraft.tiers.indexOf(tier)
  if (index === -1) editorDraft.tiers.push(tier)
  else if (editorDraft.tiers.length > 1) editorDraft.tiers.splice(index, 1)
}

function toggleEditorAspectRatio(ratio: string): void {
  const normalized = normalizeGeminiRatio(ratio)
  const index = editorDraft.aspect_ratios.indexOf(normalized)
  if (index === -1) editorDraft.aspect_ratios.push(normalized)
  else if (editorDraft.aspect_ratios.length > 1) editorDraft.aspect_ratios.splice(index, 1)
}

function addEditorAccount(accountId: number): void {
  const id = Math.trunc(Number(accountId) || 0)
  if (id > 0 && !editorDraft.account_ids.includes(id)) editorDraft.account_ids.push(id)
}

function removeEditorAccount(accountId: number): void {
  const index = editorDraft.account_ids.indexOf(accountId)
  if (index !== -1) editorDraft.account_ids.splice(index, 1)
}

function moveEditorAccount(index: number, direction: -1 | 1): void {
  const target = index + direction
  if (index < 0 || target < 0 || index >= editorDraft.account_ids.length || target >= editorDraft.account_ids.length) return
  const [item] = editorDraft.account_ids.splice(index, 1)
  editorDraft.account_ids.splice(target, 0, item)
}

function confirmRuleEditor(): void {
  if (!selectedGroupId.value) return
  if (editorPlatform.value === 'openai' && editorDraft.qualities.length === 0) {
    appStore.showError(t('admin.imageBilling.routing.editor.needQuality'))
    return
  }
  if (editorDraft.tiers.length === 0) {
    appStore.showError(t('admin.imageBilling.routing.editor.needTier'))
    return
  }
  if (editorPlatform.value === 'gemini' && editorDraft.aspect_ratios.length === 0) {
    appStore.showError(t('admin.imageBilling.routing.editor.needRatio'))
    return
  }
  if (editorDraft.account_ids.length === 0) {
    appStore.showError(t('admin.imageBilling.routing.editor.needAccount'))
    return
  }

  const key = String(selectedGroupId.value)
  if (editorPlatform.value === 'openai') {
    if (!openAIRules[key]) openAIRules[key] = []
    const rule: OpenAIRule = {
      qualities: [...editorDraft.qualities],
      tiers: [...editorDraft.tiers],
      mode: editorDraft.mode,
      account_ids: [...editorDraft.account_ids],
    }
    if (editorIndex.value === null) openAIRules[key].push(rule)
    else openAIRules[key].splice(editorIndex.value, 1, rule)
  } else {
    if (!geminiRules[key]) geminiRules[key] = []
    const rule: GeminiRule = {
      tiers: [...editorDraft.tiers],
      aspect_ratios: [...editorDraft.aspect_ratios],
      account_ids: [...editorDraft.account_ids],
    }
    if (editorIndex.value === null) geminiRules[key].push(rule)
    else geminiRules[key].splice(editorIndex.value, 1, rule)
  }
  editorOpen.value = false
  editorIndex.value = null
}

function confirmDeleteRule(index: number): void {
  deleteConfirmIndex.value = index
  deleteConfirmShow.value = true
}

function doDeleteRule(): void {
  if (!selectedGroupId.value || deleteConfirmIndex.value === null) return
  const key = String(selectedGroupId.value)
  const list = activePlatform.value === 'openai' ? openAIRules[key] : geminiRules[key]
  if (list) list.splice(deleteConfirmIndex.value, 1)
  deleteConfirmShow.value = false
  deleteConfirmIndex.value = null
}

// ===================== 计费价格 =====================
function qualityPricePlaceholder(quality: ImageQuality, tier: BillingTier): string {
  return imageQualityPricePlaceholders[quality]?.[tier] ?? ''
}

function flatPricePlaceholder(tier: BillingTier): string {
  return getImagePricePlaceholder(selectedGroup.value?.platform || '', `image_price_${tier.toLowerCase()}` as any)
}

function fillPricingDraft(): void {
  const group = selectedGroup.value
  if (!group) return
  const flat: Record<BillingTier, string> = { '1K': '', '2K': '', '4K': '' }
  flat['1K'] = group.image_price_1k === null || group.image_price_1k === undefined ? '' : String(group.image_price_1k)
  flat['2K'] = group.image_price_2k === null || group.image_price_2k === undefined ? '' : String(group.image_price_2k)
  flat['4K'] = group.image_price_4k === null || group.image_price_4k === undefined ? '' : String(group.image_price_4k)
  pricingDraft.flat_prices = flat

  const qualityPrices: ImageQualityPrices = group.image_quality_prices ?? { low: {}, medium: {}, high: {}, xhigh: {}, max: {} }
  for (const quality of imageQualities) {
    for (const tier of billingTiers) {
      const value = qualityPrices[quality]?.[tier]
      pricingDraft.quality_prices[quality][tier] = value === undefined || value === null ? '' : String(value)
    }
  }

  pricingDraft.image_rate_independent = group.image_rate_independent === true
  pricingDraft.image_rate_multiplier = String(group.image_rate_multiplier ?? 1)
}

function parsePriceInput(raw: string): { clear: boolean; value?: number; error?: boolean } {
  const text = String(raw ?? '').trim()
  if (!text) return { clear: true }
  const value = Number(text)
  if (!Number.isFinite(value) || value < 0) return { clear: false, error: true }
  return { clear: false, value }
}

async function savePricing(): Promise<void> {
  const group = selectedGroup.value
  if (!group) return

  const updates: UpdateGroupRequest = {}
  const flatFields = updates as unknown as Record<string, number | undefined>
  for (const [tier, field] of [['1K', 'image_price_1k'], ['2K', 'image_price_2k'], ['4K', 'image_price_4k']] as const) {
    const parsed = parsePriceInput(pricingDraft.flat_prices[tier])
    if (parsed.error) {
      appStore.showError(t('admin.imageBilling.pricing.invalidPrice'))
      return
    }
    // 留空 = 清除(回落默认价),后端约定负数表示清除
    flatFields[field] = parsed.clear ? -1 : parsed.value
  }

  const qualityPrices: Record<string, Record<string, number>> = {}
  for (const quality of imageQualities) {
    for (const tier of billingTiers) {
      const parsed = parsePriceInput(pricingDraft.quality_prices[quality][tier])
      if (parsed.error) {
        appStore.showError(t('admin.imageBilling.pricing.invalidPrice'))
        return
      }
      if (!parsed.clear && parsed.value !== undefined) {
        const cells = qualityPrices[quality] || (qualityPrices[quality] = {})
        cells[tier] = parsed.value
      }
    }
  }
  updates.image_quality_prices = qualityPrices as UpdateGroupRequest['image_quality_prices']

  const multiplier = Number(pricingDraft.image_rate_multiplier)
  if (!Number.isFinite(multiplier) || multiplier < 0) {
    appStore.showError(t('admin.imageBilling.pricing.invalidMultiplier'))
    return
  }
  updates.image_rate_independent = pricingDraft.image_rate_independent
  updates.image_rate_multiplier = multiplier

  savingPricing.value = true
  try {
    const updated = await groupsAPI.update(group.id, updates)
    const index = groups.value.findIndex((item) => item.id === group.id)
    if (index !== -1) groups.value[index] = updated
    fillPricingDraft()
    appStore.showSuccess(t('admin.imageBilling.pricing.saved'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.imageBilling.pricing.saveFailed'))
  } finally {
    savingPricing.value = false
  }
}

// ===================== 阈值 =====================
function validateThresholds(): boolean {
  thresholdForm.two_k_pixel_threshold = Math.trunc(Number(thresholdForm.two_k_pixel_threshold) || 0)
  thresholdForm.four_k_pixel_threshold = Math.trunc(Number(thresholdForm.four_k_pixel_threshold) || 0)
  if (thresholdForm.two_k_pixel_threshold <= 0 || thresholdForm.four_k_pixel_threshold <= 0) {
    appStore.showError(t('admin.imageBilling.thresholds.invalidThreshold'))
    return false
  }
  if (thresholdForm.four_k_pixel_threshold <= thresholdForm.two_k_pixel_threshold) {
    appStore.showError(t('admin.imageBilling.thresholds.invalid4k'))
    return false
  }
  return true
}

async function saveThresholds(): Promise<void> {
  if (!validateThresholds()) return
  savingThresholds.value = true
  try {
    const settings = await updateImageBillingThresholdSettings({ ...thresholdForm })
    thresholdForm.two_k_pixel_threshold = settings.two_k_pixel_threshold
    thresholdForm.four_k_pixel_threshold = settings.four_k_pixel_threshold
    appStore.showSuccess(t('admin.imageBilling.thresholds.saved'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.imageBilling.thresholds.saveFailed'))
  } finally {
    savingThresholds.value = false
  }
}

function resetThresholdDefaults(): void {
  thresholdForm.two_k_pixel_threshold = DEFAULT_2K_THRESHOLD
  thresholdForm.four_k_pixel_threshold = DEFAULT_4K_THRESHOLD
}

// ===================== 数据加载 =====================
async function loadRoutingAccounts(): Promise<Account[]> {
  // 后端 page_size 上限是 1000;传更大的值会被回退成默认 20,导致新账号不出现在可选列表。
  const pageSize = 1000
  const firstPage = await accountsAPI.list(1, pageSize, { lite: 'true' })
  const allAccounts = [...(firstPage.items || [])]
  const totalPages = Math.max(1, Number(firstPage.pages) || 1)
  for (let page = 2; page <= totalPages; page += 1) {
    const nextPage = await accountsAPI.list(page, pageSize, { lite: 'true' })
    allAccounts.push(...(nextPage.items || []))
  }
  return allAccounts
}

async function loadAll(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    const [thresholds, groupItems, accountItems, routing, geminiRouting] = await Promise.all([
      getImageBillingThresholdSettings(),
      groupsAPI.getAllIncludingInactive(),
      loadRoutingAccounts(),
      getImageBillingAccountRoutingSettings(),
      getGeminiImageBillingRoutingSettings(),
    ])
    thresholdForm.two_k_pixel_threshold = thresholds.two_k_pixel_threshold || DEFAULT_2K_THRESHOLD
    thresholdForm.four_k_pixel_threshold = thresholds.four_k_pixel_threshold || DEFAULT_4K_THRESHOLD
    groups.value = groupItems || []
    accounts.value = accountItems || []
    applyOpenAIRouting(routing as ImageBillingRoutingSettingsInput)
    applyGeminiRouting(geminiRouting as GeminiRoutingSettingsInput)

    if (!selectedGroup.value) {
      selectedGroupId.value = platformGroups.value[0]?.id ?? null
    }
    fillPricingDraft()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.imageBilling.loadFailed'))
  } finally {
    loading.value = false
  }
}

function switchPlatform(platform: 'openai' | 'gemini'): void {
  if (activePlatform.value === platform) return
  activePlatform.value = platform
  const current = platformGroups.value.find((group) => group.id === selectedGroupId.value)
  if (!current) {
    selectedGroupId.value = platformGroups.value[0]?.id ?? null
  }
}

watch([selectedGroupId, activePlatform], () => {
  fillPricingDraft()
})

// 初始加载
void loadAll()
</script>
