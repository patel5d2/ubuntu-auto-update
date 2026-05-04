import { StatusBar } from 'expo-status-bar';
import { StyleSheet, Text, View, SafeAreaView, TouchableOpacity, FlatList } from 'react-native';
import { useState } from 'react';

// Mock data for the initial skeleton
const MOCK_HOSTS = [
  { id: '1', hostname: 'web-prod-01', status: 'online', upToDate: true },
  { id: '2', hostname: 'db-prod-01', status: 'online', upToDate: false },
  { id: '3', hostname: 'worker-01', status: 'offline', upToDate: false },
];

export default function App() {
  const [hosts, setHosts] = useState(MOCK_HOSTS);

  const renderHost = ({ item }: { item: typeof MOCK_HOSTS[0] }) => (
    <View style={styles.hostCard}>
      <View>
        <Text style={styles.hostname}>{item.hostname}</Text>
        <Text style={styles.status}>
          {item.status === 'online' ? '🟢 Online' : '🔴 Offline'}
        </Text>
      </View>
      <View style={styles.updateBadgeContainer}>
        {item.upToDate ? (
          <View style={[styles.badge, styles.badgeSuccess]}>
            <Text style={styles.badgeText}>Up to date</Text>
          </View>
        ) : (
          <View style={[styles.badge, styles.badgeWarning]}>
            <Text style={styles.badgeTextWarning}>Updates pending</Text>
          </View>
        )}
      </View>
    </View>
  );

  return (
    <SafeAreaView style={styles.container}>
      <StatusBar style="auto" />
      
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Ubuntu Auto-Update</Text>
        <Text style={styles.headerSubtitle}>Fleet Overview</Text>
      </View>

      <View style={styles.statsContainer}>
        <View style={styles.statBox}>
          <Text style={styles.statValue}>{hosts.length}</Text>
          <Text style={styles.statLabel}>Total Hosts</Text>
        </View>
        <View style={styles.statBox}>
          <Text style={styles.statValue}>
            {hosts.filter(h => !h.upToDate).length}
          </Text>
          <Text style={styles.statLabel}>Pending Updates</Text>
        </View>
      </View>

      <Text style={styles.sectionTitle}>Recent Hosts</Text>
      
      <FlatList
        data={hosts}
        keyExtractor={item => item.id}
        renderItem={renderHost}
        contentContainerStyle={styles.listContainer}
      />

      <TouchableOpacity style={styles.fab}>
        <Text style={styles.fabText}>Approve Updates</Text>
      </TouchableOpacity>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#f3f4f6',
  },
  header: {
    padding: 20,
    paddingTop: 40,
    backgroundColor: '#fff',
    borderBottomWidth: 1,
    borderBottomColor: '#e5e7eb',
  },
  headerTitle: {
    fontSize: 24,
    fontWeight: 'bold',
    color: '#111827',
  },
  headerSubtitle: {
    fontSize: 16,
    color: '#6b7280',
    marginTop: 4,
  },
  statsContainer: {
    flexDirection: 'row',
    padding: 16,
    gap: 16,
  },
  statBox: {
    flex: 1,
    backgroundColor: '#fff',
    padding: 16,
    borderRadius: 12,
    alignItems: 'center',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 1 },
    shadowOpacity: 0.05,
    shadowRadius: 2,
    elevation: 2,
  },
  statValue: {
    fontSize: 24,
    fontWeight: 'bold',
    color: '#e95420', // Ubuntu Orange
  },
  statLabel: {
    fontSize: 14,
    color: '#6b7280',
    marginTop: 4,
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: '600',
    color: '#374151',
    marginLeft: 16,
    marginTop: 8,
    marginBottom: 8,
  },
  listContainer: {
    padding: 16,
    paddingTop: 0,
    gap: 12,
  },
  hostCard: {
    backgroundColor: '#fff',
    padding: 16,
    borderRadius: 12,
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 1 },
    shadowOpacity: 0.05,
    shadowRadius: 2,
    elevation: 2,
  },
  hostname: {
    fontSize: 16,
    fontWeight: '600',
    color: '#111827',
  },
  status: {
    fontSize: 14,
    color: '#6b7280',
    marginTop: 4,
  },
  updateBadgeContainer: {
    alignItems: 'flex-end',
  },
  badge: {
    paddingHorizontal: 8,
    paddingVertical: 4,
    borderRadius: 12,
  },
  badgeSuccess: {
    backgroundColor: '#def7ec',
  },
  badgeWarning: {
    backgroundColor: '#fef3c7',
  },
  badgeText: {
    color: '#03543f',
    fontSize: 12,
    fontWeight: '500',
  },
  badgeTextWarning: {
    color: '#92400e',
    fontSize: 12,
    fontWeight: '500',
  },
  fab: {
    position: 'absolute',
    bottom: 32,
    right: 24,
    left: 24,
    backgroundColor: '#e95420', // Ubuntu Orange
    paddingVertical: 16,
    borderRadius: 12,
    alignItems: 'center',
    shadowColor: '#e95420',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 8,
    elevation: 5,
  },
  fabText: {
    color: '#fff',
    fontSize: 16,
    fontWeight: '600',
  },
});
