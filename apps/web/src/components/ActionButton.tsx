import type { ButtonHTMLAttributes, ReactNode } from 'react'

type ActionButtonVariant = 'primary' | 'secondary' | 'danger' | 'quiet'

type ActionButtonProps = {
  children: ReactNode
  variant?: ActionButtonVariant
  isLoading?: boolean
  isDisabled?: boolean
  loadingLabel?: string
} & Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'disabled'>

const variantClasses: Record<ActionButtonVariant, string> = {
  primary: 'rounded-[10px] bg-[#7c3aed] text-white hover:bg-[#6d28d9] disabled:bg-[#7c3aed]/60 shadow-[0_1px_3px_rgba(124,58,237,0.3)]',
  secondary: 'rounded-[10px] border border-[#ebebeb] bg-white text-[#292929] hover:border-[#a78bfa] hover:bg-[#faf5ff] disabled:border-[#ebebeb] disabled:bg-[#f7f7f7] disabled:text-[#8e8e8e]',
  danger: 'rounded-[10px] bg-red-600 text-white hover:bg-red-500 disabled:bg-red-600/60',
  quiet: 'rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] text-[#5e5e5e] hover:border-[#d4cce4] hover:bg-[#faf5ff] disabled:text-[#8e8e8e]'
}

export function ActionButton({
  children,
  variant = 'secondary',
  isLoading = false,
  isDisabled = false,
  loadingLabel,
  className = '',
  ...props
}: ActionButtonProps) {
  const disabled = isDisabled || isLoading
  return (
    <button
      {...props}
      disabled={disabled}
      aria-busy={isLoading}
      className={`px-3 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed ${variantClasses[variant]} ${className}`.trim()}
    >
      {isLoading ? loadingLabel ?? 'Выполняется...' : children}
    </button>
  )
}
