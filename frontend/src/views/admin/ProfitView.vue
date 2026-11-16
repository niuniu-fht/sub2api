<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6">
      <!-- 页头 + 时间筛选 -->
      <div class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.title') }}</h1>
            <p class="mt-1 truncate text-sm text-gray-500 dark:text-gray-400">{{ t('admin.profit.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button
              v-for="preset in rangePresets"
              :key="preset.days"
              type="button"
              class="rounded-lg px-3 py-1.5 text-sm font-medium transition"
              :class="activePreset === preset.days
                ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-200'
                : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700'"
              @click="applyPreset(preset.days)"
            >
              {{ preset.label }}
            </button>
            <input
              v-model="filterStartDate"
              type="date"
              class="rounded-lg border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              @change="applyCustomRange"
            />
            <span class="text-gray-400">~</span>
            <input
              v-model="filterEndDate"
              type="date"
              class="rounded-lg border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              @change="applyCustomRange"
            />
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="reload">
              {{ t('admin.profit.refresh') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" @click="settingsOpen = true">
              {{ t('admin.profit.settings.button') }}
            </button>
          </div>
        </div>
      </div>

      <!-- 汇总卡片 -->
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div
          v-for="card in summaryCards"
          :key="card.key"
          class="rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-1 text-2xl font-semibold tabular-nums" :class="card.valueClass">
            {{ card.value }}
          </div>
          <div v-if="card.hint" class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ card.hint }}</div>
        </div>
      </div>

      <!-- 成本配置覆盖率警告 -->
      <div
        v-if="summary && summary.totals.requests > 0 && costCoverage < 1"
        class="rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-200"
      >
        {{ t('admin.profit.costCoverageWarn', { pct: Math.round(costCoverage * 100) }) }}
        <button type="button" class="ml-1 font-medium underline" @click="router.push('/admin/profit/cost')">
          {{ t('admin.profit.goPricing') }}
        </button>
      </div>

      <!-- 每日趋势 -->
      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <h2 class="mb-3 text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.dailyTrend') }}</h2>
          <div v-if="daily.length" class="h-64">
            <Line :data="chartData" :options="chartOptions" :plugins="linePlugins" />
          </div>
        <div v-else class="flex h-40 items-center justify-center text-sm text-gray-400">
          {{ loading ? t('common.loading') : t('admin.profit.noData') }}
        </div>
      </div>

      <!-- Tabs -->
      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="mb-4 flex flex-wrap items-center gap-3">
          <div class="flex flex-wrap gap-2 rounded-xl bg-gray-100 p-1 dark:bg-dark-900">
            <button
              v-for="tab in tabs"
              :key="tab.key"
              type="button"
              class="rounded-lg px-4 py-2 text-sm font-medium transition"
              :class="activeTab === tab.key
                ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-200'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="activeTab = tab.key"
            >
              {{ tab.label }}
            </button>
          </div>
          <div v-if="activeTab === 'users'" class="ml-auto flex items-center gap-2">
            <input
              v-model="userSearch"
              type="text"
              class="w-56 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              :placeholder="t('admin.profit.searchUser')"
              @keyup.enter="loadUsers"
            />
            <select
              v-model="userOrder"
              class="rounded-lg border border-gray-300 bg-white px-2 py-2 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
              @change="loadUsers"
            >
              <option value="profit_desc">{{ t('admin.profit.order.profitDesc') }}</option>
              <option value="profit_asc">{{ t('admin.profit.order.profitAsc') }}</option>
              <option value="revenue_desc">{{ t('admin.profit.order.revenueDesc') }}</option>
              <option value="cost_desc">{{ t('admin.profit.order.costDesc') }}</option>
            </select>
          </div>
        </div>

        <!-- 用户表 -->
        <div v-if="activeTab === 'users'" class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2">{{ t('admin.profit.col.user') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.rate') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.requests') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.images') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.revenue') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.cost') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.profit') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.margin') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in userRows"
                :key="row.user_id"
                class="cursor-pointer border-b border-gray-100 transition hover:bg-gray-50 dark:border-dark-700/50 dark:hover:bg-dark-700/40"
                @click="openUserDetail(row.user_id)"
              >
                <td class="px-3 py-2.5">
                  <div class="font-medium text-gray-900 dark:text-white">{{ row.email }}</div>
                  <div v-if="row.name" class="text-xs text-gray-400">{{ row.name }}</div>
                </td>
                <td class="px-3 py-2.5 text-right tabular-nums">
                  <span
                    class="rounded-md px-1.5 py-0.5 text-xs font-medium"
                    :class="row.rate_set ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-200' : 'text-gray-400'"
                  >
                    {{ (row.rate * 10).toFixed(1) }}{{ t('admin.profit.zhe') }}
                  </span>
                </td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtInt(row.requests) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtInt(row.images) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtMoney(row.revenue) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtMoney(row.cost) }}</td>
                <td class="px-3 py-2.5 text-right font-semibold tabular-nums" :class="profitClass(row.profit)">
                  {{ fmtMoney(row.profit) }}
                </td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-500">{{ fmtPct(row.margin) }}</td>
              </tr>
              <tr v-if="!userRows.length && !loading">
                <td colspan="8" class="px-3 py-8 text-center text-gray-400">{{ t('admin.profit.noData') }}</td>
              </tr>
            </tbody>
          </table>
          <!-- 分页 -->
          <div v-if="userTotal > userPageSize" class="mt-4 flex items-center justify-between text-sm text-gray-500">
            <span>{{ t('admin.profit.paged', { total: userTotal }) }}</span>
            <div class="flex items-center gap-2">
              <button class="btn btn-secondary btn-sm" :disabled="userPage <= 1" @click="userPage--; loadUsers()">
                {{ t('admin.profit.prevPage') }}
              </button>
              <span class="tabular-nums">{{ userPage }} / {{ Math.ceil(userTotal / userPageSize) }}</span>
              <button
                class="btn btn-secondary btn-sm"
                :disabled="userPage >= Math.ceil(userTotal / userPageSize)"
                @click="userPage++; loadUsers()"
              >
                {{ t('admin.profit.nextPage') }}
              </button>
            </div>
          </div>
        </div>

        <!-- 模型表 -->
        <div v-else-if="activeTab === 'models'" class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2">{{ t('admin.profit.col.model') }}</th>
                <th class="px-3 py-2">{{ t('admin.profit.col.platform') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.requests') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.images') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.revenue') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.cost') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.profit') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.margin') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in modelRows" :key="row.model + row.platform" class="border-b border-gray-100 dark:border-dark-700/50">
                <td class="px-3 py-2.5 font-medium text-gray-900 dark:text-white">{{ row.model }}</td>
                <td class="px-3 py-2.5 text-gray-500">{{ row.platform || '—' }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtInt(row.requests) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtInt(row.images) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtMoney(row.revenue) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtMoney(row.cost) }}</td>
                <td class="px-3 py-2.5 text-right font-semibold tabular-nums" :class="profitClass(row.profit)">{{ fmtMoney(row.profit) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-500">{{ fmtPct(row.margin) }}</td>
              </tr>
              <tr v-if="!modelRows.length && !loading">
                <td colspan="8" class="px-3 py-8 text-center text-gray-400">{{ t('admin.profit.noData') }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 账号表 -->
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2">{{ t('admin.profit.col.account') }}</th>
                <th class="px-3 py-2">{{ t('admin.profit.col.platform') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.requests') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.images') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.revenue') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.cost') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.profit') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in accountRows" :key="row.account_id" class="border-b border-gray-100 dark:border-dark-700/50">
                <td class="px-3 py-2.5 font-medium text-gray-900 dark:text-white">
                  {{ row.account_name || row.account_id }}
                </td>
                <td class="px-3 py-2.5 text-gray-500">{{ row.platform || '—' }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtInt(row.requests) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtInt(row.images) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtMoney(row.revenue) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ fmtMoney(row.cost) }}</td>
                <td class="px-3 py-2.5 text-right font-semibold tabular-nums" :class="profitClass(row.profit)">{{ fmtMoney(row.profit) }}</td>
              </tr>
              <tr v-if="!accountRows.length && !loading">
                <td colspan="7" class="px-3 py-8 text-center text-gray-400">{{ t('admin.profit.noData') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- 用户详情抽屉 -->
    <div
      v-if="detailOpen"
      class="fixed inset-0 z-50 flex justify-end bg-black/40"
      @click.self="detailOpen = false"
    >
      <div class="h-full w-full max-w-3xl overflow-y-auto bg-white p-6 shadow-2xl dark:bg-dark-800">
        <div class="mb-4 flex items-start justify-between">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ detail?.totals ? detailEmail : '' }}
            </h2>
            <p class="mt-1 text-sm text-gray-500">
              {{ t('admin.profit.detail.rate') }}:
              <span class="font-medium text-gray-700 dark:text-gray-200">{{ detailRate }}</span>
            </p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" @click="detailOpen = false">
            {{ t('common.close') }}
          </button>
        </div>

        <template v-if="detail">
          <!-- 用户汇总 -->
          <div class="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-900/60">
              <div class="text-xs text-gray-500">{{ t('admin.profit.col.requests') }}</div>
              <div class="mt-1 text-lg font-semibold tabular-nums">{{ fmtInt(detail.totals.requests) }}</div>
            </div>
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-900/60">
              <div class="text-xs text-gray-500">{{ t('admin.profit.col.revenue') }}</div>
              <div class="mt-1 text-lg font-semibold tabular-nums">{{ fmtMoney(detail.totals.revenue) }}</div>
            </div>
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-900/60">
              <div class="text-xs text-gray-500">{{ t('admin.profit.col.cost') }}</div>
              <div class="mt-1 text-lg font-semibold tabular-nums">{{ fmtMoney(detail.totals.cost) }}</div>
            </div>
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-900/60">
              <div class="text-xs text-gray-500">{{ t('admin.profit.col.profit') }}</div>
              <div class="mt-1 text-lg font-semibold tabular-nums" :class="profitClass(detail.totals.revenue - detail.totals.cost)">
                {{ fmtMoney(detail.totals.revenue - detail.totals.cost) }}
              </div>
            </div>
          </div>

          <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.detail.byModel') }}</h3>
          <div class="mb-5 overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
            <table class="min-w-full text-sm">
              <thead>
                <tr class="border-b border-gray-200 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
                  <th class="px-3 py-2">{{ t('admin.profit.col.model') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.requests') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.images') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.revenue') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.cost') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.profit') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="m in detail.by_model" :key="m.model" class="border-b border-gray-100 dark:border-dark-700/50">
                  <td class="px-3 py-2 font-medium text-gray-900 dark:text-white">{{ m.model }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtInt(m.requests) }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtInt(m.images) }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtMoney(m.revenue) }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtMoney(m.cost) }}</td>
                  <td class="px-3 py-2 text-right font-semibold tabular-nums" :class="profitClass(m.profit)">{{ fmtMoney(m.profit) }}</td>
                </tr>
                <tr v-if="!detail.by_model.length">
                  <td colspan="6" class="px-3 py-6 text-center text-gray-400">{{ t('admin.profit.noData') }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.detail.byAccount') }}</h3>
          <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
            <table class="min-w-full text-sm">
              <thead>
                <tr class="border-b border-gray-200 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
                  <th class="px-3 py-2">{{ t('admin.profit.col.account') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.requests') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.images') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.revenue') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.cost') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.profit.col.profit') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="a in detail.by_account" :key="a.account_id" class="border-b border-gray-100 dark:border-dark-700/50">
                  <td class="px-3 py-2 font-medium text-gray-900 dark:text-white">{{ a.account_name || a.account_id }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtInt(a.requests) }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtInt(a.images) }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtMoney(a.revenue) }}</td>
                  <td class="px-3 py-2 text-right tabular-nums">{{ fmtMoney(a.cost) }}</td>
                  <td class="px-3 py-2 text-right font-semibold tabular-nums" :class="profitClass(a.profit)">{{ fmtMoney(a.profit) }}</td>
                </tr>
                <tr v-if="!detail.by_account.length">
                  <td colspan="6" class="px-3 py-6 text-center text-gray-400">{{ t('admin.profit.noData') }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <h3 class="mb-2 mt-5 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.dailyTrend') }}</h3>
          <div v-if="detail.daily.length" class="h-56">
            <Line :data="detailChartData" :options="chartOptions" :plugins="linePlugins" />
          </div>
        </template>
      </div>
    </div>

    <!-- 设置抽屉：默认折扣率 + 用户费率管理 -->
    <div v-if="settingsOpen" class="fixed inset-0 z-50 flex justify-end bg-black/40" @click.self="settingsOpen = false">
      <div class="h-full w-full max-w-xl overflow-y-auto bg-white p-6 shadow-2xl dark:bg-dark-800">
        <div class="mb-5 flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.settings.title') }}</h2>
          <button type="button" class="btn btn-secondary btn-sm" @click="settingsOpen = false">{{ t('common.close') }}</button>
        </div>

        <div class="mb-6 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.profit.settings.defaultRate') }}
          </label>
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.profit.settings.defaultRateHint') }}</p>
          <div class="mt-2 flex items-center gap-2">
            <input
              v-model.number="defaultRateInput"
              type="number"
              step="0.05"
              min="0.05"
              max="10"
              class="w-32 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            />
            <button type="button" class="btn btn-primary btn-sm" :disabled="savingConfig" @click="saveDefaultRate">
              {{ t('common.save') }}
            </button>
          </div>
        </div>

        <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.settings.userRates') }}</h3>
        <div class="mb-3 flex flex-wrap items-end gap-2">
          <div>
            <label class="text-xs text-gray-500">{{ t('admin.profit.settings.userId') }}</label>
            <input
              v-model.number="newRateUserId"
              type="number"
              class="mt-1 w-28 rounded-lg border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            />
          </div>
          <div>
            <label class="text-xs text-gray-500">{{ t('admin.profit.settings.rate') }}</label>
            <input
              v-model.number="newRateValue"
              type="number"
              step="0.05"
              min="0.05"
              max="10"
              class="mt-1 w-24 rounded-lg border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            />
          </div>
          <div class="flex-1">
            <label class="text-xs text-gray-500">{{ t('admin.profit.settings.note') }}</label>
            <input
              v-model="newRateNote"
              type="text"
              class="mt-1 w-full rounded-lg border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            />
          </div>
          <button type="button" class="btn btn-primary btn-sm" @click="addUserRate">{{ t('common.add') }}</button>
        </div>

        <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
                <th class="px-3 py-2">{{ t('admin.profit.col.user') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.profit.col.rate') }}</th>
                <th class="px-3 py-2">{{ t('admin.profit.settings.note') }}</th>
                <th class="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in userRates" :key="r.user_id" class="border-b border-gray-100 dark:border-dark-700/50">
                <td class="px-3 py-2">
                  <div class="font-medium text-gray-900 dark:text-white">{{ r.email }}</div>
                  <div class="text-xs text-gray-400">ID {{ r.user_id }}</div>
                </td>
                <td class="px-3 py-2 text-right tabular-nums">{{ r.rate }}</td>
                <td class="px-3 py-2 text-gray-500">{{ r.note }}</td>
                <td class="px-3 py-2 text-right">
                  <button
                    type="button"
                    class="text-xs font-medium text-red-600 hover:underline dark:text-red-400"
                    @click="removeUserRate(r.user_id)"
                  >
                    {{ t('common.delete') }}
                  </button>
                </td>
              </tr>
              <tr v-if="!userRates.length">
                <td colspan="4" class="px-3 py-6 text-center text-gray-400">{{ t('admin.profit.settings.noRates') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Chart as ChartJS, CategoryScale, Filler, Legend, LineElement, LinearScale, PointElement, Tooltip, type ChartOptions } from 'chart.js'
import { Line } from 'vue-chartjs'
import ChartDataLabels from 'chartjs-plugin-datalabels'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import {
  getProfitSummary,
  getProfitUsers,
  getProfitUserDetail,
  getProfitModels,
  getProfitAccounts,
  getProfitRates,
  updateProfitRate,
  deleteProfitRate,
  getProfitConfig,
  updateProfitConfig,
  type ProfitSummaryResponse,
  type ProfitUserRow,
  type ProfitModelRow,
  type ProfitAccountRow,
  type UserProfitDetailResponse,
  type ProfitUserRate,
} from '@/api/admin/profit'

ChartJS.register(Tooltip, Legend, LineElement, LinearScale, PointElement, CategoryScale, Filler)

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const loading = ref(false)
const activePreset = ref(7)
const filterStartDate = ref('')
const filterEndDate = ref('')
const summary = ref<ProfitSummaryResponse | null>(null)
const activeTab = ref<'users' | 'models' | 'accounts'>('users')

const userRows = ref<ProfitUserRow[]>([])
const userTotal = ref(0)
const userPage = ref(1)
const userPageSize = 20
const userSearch = ref('')
const userOrder = ref('profit_desc')
const modelRows = ref<ProfitModelRow[]>([])
const accountRows = ref<ProfitAccountRow[]>([])

const detailOpen = ref(false)
const detail = ref<UserProfitDetailResponse | null>(null)
const detailEmail = ref('')

const settingsOpen = ref(false)
const defaultRateInput = ref(1)
const savingConfig = ref(false)
const userRates = ref<ProfitUserRate[]>([])
const newRateUserId = ref<number | null>(null)
const newRateValue = ref(0.9)
const newRateNote = ref('')

const tabs = computed(() => [
  { key: 'users' as const, label: t('admin.profit.tabs.users') },
  { key: 'models' as const, label: t('admin.profit.tabs.models') },
  { key: 'accounts' as const, label: t('admin.profit.tabs.accounts') },
])

const rangePresets = computed(() => [
  { days: 1, label: t('admin.profit.range.today') },
  { days: 7, label: t('admin.profit.range.days7') },
  { days: 30, label: t('admin.profit.range.days30') },
  { days: 90, label: t('admin.profit.range.days90') },
])

function fmtDate(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

function applyPreset(days: number) {
  activePreset.value = days
  const end = new Date()
  const start = new Date(end.getTime() - days * 86400000)
  filterStartDate.value = fmtDate(start)
  filterEndDate.value = fmtDate(end)
  reload()
}

function applyCustomRange() {
  if (filterStartDate.value && filterEndDate.value) {
    activePreset.value = 0
    reload()
  }
}

function rangeParams() {
  return { start: filterStartDate.value || undefined, end: filterEndDate.value || undefined }
}

const costCoverage = computed(() => {
  if (!summary.value || !summary.value.totals.requests) return 1
  return summary.value.totals.configured_cost_requests / summary.value.totals.requests
})

const summaryCards = computed(() => {
  const totals = summary.value?.totals
  const revenue = totals?.revenue ?? 0
  const cost = totals?.cost ?? 0
  const profit = revenue - cost
  const margin = revenue > 0 ? profit / revenue : 0
  return [
    { key: 'revenue', label: t('admin.profit.cards.revenue'), value: fmtMoney(revenue), valueClass: 'text-gray-900 dark:text-white', hint: totals ? t('admin.profit.cards.gross', { v: fmtMoney(totals.gross_revenue) }) : '' },
    { key: 'cost', label: t('admin.profit.cards.cost'), value: fmtMoney(cost), valueClass: 'text-gray-900 dark:text-white', hint: totals ? t('admin.profit.cards.configured', { pct: Math.round(costCoverage.value * 100) }) : '' },
    { key: 'profit', label: t('admin.profit.cards.profit'), value: fmtMoney(profit), valueClass: profitClass(profit), hint: totals ? t('admin.profit.cards.requests', { n: fmtInt(totals.requests), img: fmtInt(totals.image_count) }) : '' },
    { key: 'margin', label: t('admin.profit.cards.margin'), value: fmtPct(margin), valueClass: profitClass(profit), hint: '' },
  ]
})

function profitClass(v: number): string {
  if (v > 0) return 'text-emerald-600 dark:text-emerald-400'
  if (v < 0) return 'text-red-600 dark:text-red-400'
  return 'text-gray-900 dark:text-white'
}

function fmtMoney(v: number): string {
  return `¥${Number(v || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function fmtInt(v: number): string {
  return Number(v || 0).toLocaleString('zh-CN')
}

function fmtPct(v: number): string {
  return `${(Number(v || 0) * 100).toFixed(1)}%`
}

const daily = computed(() => summary.value?.daily ?? [])

const chartData = computed(() => buildChartData(daily.value))
const detailChartData = computed(() => buildChartData(detail.value?.daily ?? []))

function buildChartData(points: { day: string; revenue: number; cost: number; profit: number }[]) {
  return {
    labels: points.map((p) => p.day.slice(5)),
    datasets: [
      {
        label: t('admin.profit.chart.revenue'),
        data: points.map((p) => p.revenue),
        borderColor: '#3b82f6',
        backgroundColor: '#3b82f620',
        fill: true,
        tension: 0.3,
        pointRadius: 2,
      },
      {
        label: t('admin.profit.chart.cost'),
        data: points.map((p) => p.cost),
        borderColor: '#f59e0b',
        backgroundColor: '#f59e0b20',
        fill: true,
        tension: 0.3,
        pointRadius: 2,
      },
      {
        label: t('admin.profit.chart.profit'),
        data: points.map((p) => p.profit),
        borderColor: '#10b981',
        backgroundColor: '#10b98120',
        fill: false,
        tension: 0.3,
        pointRadius: 2,
      },
    ],
  }
}

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  plugins: {
    legend: { position: 'top' as const },
    // 默认在数据点旁显示数值；点数过多（近90天）时自动隐藏避免重叠
    datalabels: {
      display: (ctx: any) => (ctx.dataset.data as number[]).length <= 32,
      formatter: (v: number) => fmtChartLabel(v),
      font: { size: 9 },
      color: '#9ca3af',
      align: 'top',
      anchor: 'end',
      clamp: true,
    },
  },
  scales: { y: { beginAtZero: false } },
} as ChartOptions<'line'>

// 数据标签只作用于本页折线图（局部插件，避免影响其他页面的 chart.js 实例）
const linePlugins = [ChartDataLabels]

function fmtChartLabel(v: number): string {
  const n = Number(v) || 0
  if (n === 0) return ''
  if (Math.abs(n) >= 10000) return `${(n / 1000).toFixed(0)}k`
  if (Math.abs(n) >= 1000) return `${(n / 1000).toFixed(1)}k`
  return n.toFixed(1)
}

async function reload() {
  loading.value = true
  try {
    const params = rangeParams()
    // 四路并行加载，避免串行等待
    const [s, models, accounts, users] = await Promise.all([
      getProfitSummary(params),
      getProfitModels(params),
      getProfitAccounts(params),
      getProfitUsers({
        ...params,
        search: userSearch.value || undefined,
        order: userOrder.value,
        page: userPage.value,
        page_size: userPageSize,
      }),
    ])
    summary.value = s
    modelRows.value = models.items
    accountRows.value = accounts.items
    userRows.value = users.items
    userTotal.value = users.total
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    loading.value = false
  }
}

async function loadUsers() {
  try {
    const res = await getProfitUsers({
      ...rangeParams(),
      search: userSearch.value || undefined,
      order: userOrder.value,
      page: userPage.value,
      page_size: userPageSize,
    })
    userRows.value = res.items
    userTotal.value = res.total
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  }
}

async function openUserDetail(userId: number) {
  const row = userRows.value.find((r) => r.user_id === userId)
  detailEmail.value = row?.email || `#${userId}`
  detailOpen.value = true
  detail.value = null
  try {
    detail.value = await getProfitUserDetail(userId, rangeParams())
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  }
}

const detailRate = computed(() => {
  if (!detail.value) return ''
  return `${(detail.value.rate * 10).toFixed(1)}${t('admin.profit.zhe')}${detail.value.rate_set ? '' : t('admin.profit.defaultSuffix')}`
})

async function loadSettings() {
  try {
    const [cfg, rates] = await Promise.all([getProfitConfig(), getProfitRates()])
    defaultRateInput.value = cfg.default_user_rate || 1
    userRates.value = rates.items
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  }
}

async function saveDefaultRate() {
  if (!defaultRateInput.value || defaultRateInput.value <= 0 || defaultRateInput.value > 10) {
    appStore.showError(t('admin.profit.settings.rateInvalid'))
    return
  }
  savingConfig.value = true
  try {
    await updateProfitConfig(defaultRateInput.value)
    appStore.showSuccess(t('admin.profit.settings.saved'))
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  } finally {
    savingConfig.value = false
  }
}

async function addUserRate() {
  if (!newRateUserId.value || !newRateValue.value || newRateValue.value <= 0 || newRateValue.value > 10) {
    appStore.showError(t('admin.profit.settings.rateInvalid'))
    return
  }
  try {
    await updateProfitRate(newRateUserId.value, newRateValue.value, newRateNote.value || '')
    appStore.showSuccess(t('admin.profit.settings.saved'))
    newRateUserId.value = null
    newRateNote.value = ''
    await loadSettings()
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  }
}

async function removeUserRate(userId: number) {
  try {
    await deleteProfitRate(userId)
    appStore.showSuccess(t('admin.profit.settings.saved'))
    await loadSettings()
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message || t('errors.networkError'))
  }
}

watch(activeTab, (tab) => {
  if (tab === 'users' && !userRows.value.length) loadUsers()
})

onMounted(() => {
  applyPreset(1)
  loadSettings()
})
</script>
