/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  createChart,
  type BaseChartProps,
} from '@visactor/react-vchart/esm/charts/BaseChart'
import VChartCore from '@visactor/vchart/esm/vchart-simple'
export type { ISpec } from '@visactor/vchart/esm/core'

// Dashboard specs use area/bar/pie (plus line/common), all registered by the
// library's simple preset. Avoid registering every other chart at import time.
export const VChart = createChart<BaseChartProps>('VChart', {
  vchartConstructor: VChartCore,
})

export const VCHART_OPTION = {
  // 与老前端保持一致（浏览器环境渲染优化）
  mode: 'desktop-browser',
} as const
