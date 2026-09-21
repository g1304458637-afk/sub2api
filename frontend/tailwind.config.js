/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // MUC Harness: 主色调 - 民大红（中央民族大学校色）
        primary: {
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
