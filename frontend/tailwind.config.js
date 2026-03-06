/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        primary: {
          20: '#CCFCFF',
          30: '#61F0FE',
          40: '#40C4DE',
          50: '#019AB8',
          60: '#202E3C',
          70: '#151E27',
          80: '#101820'
        },
        gray: {
          50: '#808B9B',
          60: '#4B5565'
        },
        success: {
          40: '#47CD89',
          90: '#11322D'
        },
        error: {
          40: '#F97066',
          90: '#381D1E'
        },
        warning: {
          40: '#CDA747',
          90: '#322D11'
        }
      },
      fontFamily: {
        space: ['Space Grotesk', 'sans-serif'],
        sans: ['Inter', 'system-ui', 'sans-serif']
      },
      fontSize: {
        10: ['0.625rem', { lineHeight: '1.25' }],
        11: ['0.6875rem', { lineHeight: '1.25' }],
        12: ['0.75rem', { lineHeight: '1.25' }],
        13: ['0.8125rem', { lineHeight: '1.25' }],
        14: ['0.875rem', { lineHeight: '1.25' }],
        15: ['0.9375rem', { lineHeight: '1.25' }],
        16: ['1rem', { lineHeight: '1.25' }],
        18: ['1.125rem', { lineHeight: '1.25' }],
        20: ['1.25rem', { lineHeight: '1.25' }],
        22: ['1.375rem', { lineHeight: '1.25' }],
        24: ['1.5rem', { lineHeight: '1.25' }]
      }
    }
  },
  plugins: []
}
