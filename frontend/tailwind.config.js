/** @type {import('tailwindcss').Config} */

// 校园品牌调色板：构建期 BRAND env 决定（默认 muc，历史构建零变化）。
// hubu 深湖大绿锚点采样自湖北大学主视觉图（横幅深绿 #17503A/#104B37、青铜金 #BC9D53），
// 为采样值而非湖北大学官方 VI 标准色。
const PALETTES = {
  muc: {
    50: '#fdf3f3',
    100: '#fcdcda',
    200: '#f9b9b9',
    300: '#f28d8d',
    400: '#e75758',
    500: '#c92a2b',
    600: '#ac0e0f',
    700: '#8f0c0d',
    800: '#771012',
    900: '#631215',
    950: '#380608'
  },
  hubu: {
    50: '#eaf6f0',
    100: '#c7e8da',
    200: '#93d2bb',
    300: '#57b493',
    400: '#2e9070',
    500: '#1b6e54',
    600: '#135440',
    700: '#0f4433',
    800: '#0c3629',
    900: '#0a2b21',
    950: '#051711'
  }
}

const BRAND = process.env.BRAND || 'muc'

export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 校园品牌主色调（BRAND=muc 民大红 / BRAND=hubu 深湖大绿）
        primary: PALETTES[BRAND] || PALETTES.muc,
        // 校园品牌辅色：青铜金（楚文化饰线）
        bronze: {
          50: '#fbf8ef',
          100: '#f6eed8',
          200: '#ecdbac',
          300: '#e0c57c',
          400: '#d3af5e',
          500: '#bc9d53',
          600: '#a5813b',
          700: '#87672f',
          800: '#6c5227',
          900: '#523e1f',
          950: '#2f2411'
        },
        // 辅助色 - 深蓝灰
        accent: {
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617'
        },
        // 深色模式背景 —— MUCODE 民大版 Dark Brand Theme：
        // 全站 dark:* variant 由此调色板承载（与 muc-tokens.css 的 --muc-* 同源）。
        // 950=页面底 #070708；900=Shell；800=surface/卡片；700/600=边框/悬停。
        dark: {
          50: '#f4f4f5',
          100: '#e4e4e7',
          200: '#c6c6cd',
          300: '#a9a9b2',
          400: '#8b8b93',
          500: '#6b6b73',
          600: '#3c3c44',
          700: '#26262b',
          800: '#17171a',
          900: '#0e0e10',
          950: '#070708'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        glow: '0 0 20px rgba(200, 36, 51, 0.25)',
        'glow-lg': '0 0 40px rgba(226, 56, 72, 0.35)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #c92a2b 0%, #ac0e0f 100%)',
        'gradient-dark': 'linear-gradient(135deg, #17171a 0%, #070708 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        // 功能页背景装饰（dark 下可见）：民大红低饱和 radial，克制不抢信息
        'mesh-gradient':
          'radial-gradient(at 75% 0%, rgba(200, 36, 51, 0.09) 0px, transparent 36%), radial-gradient(at 10% 80%, rgba(127, 22, 34, 0.07) 0px, transparent 30%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(200, 36, 51, 0.25)' },
          '100%': { boxShadow: '0 0 30px rgba(226, 56, 72, 0.4)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
