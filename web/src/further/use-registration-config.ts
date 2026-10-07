import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export function useFurtherRegistrationConfig() {
  return useQuery({
    queryKey: ['further-registration-config'],
    queryFn: async () => {
      const result = await api.get<{ success: boolean; data: { enabled: boolean; local_registration_enabled: boolean; registration_code_required: boolean; linuxdo_registration_enabled: boolean } }>('/api/further/registration/config')
      return requireServerSuccess(result.data).data
    },
    staleTime: 30_000,
  })
}
