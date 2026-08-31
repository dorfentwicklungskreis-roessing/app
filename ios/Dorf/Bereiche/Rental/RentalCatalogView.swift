import SwiftUI

/// „Maschinchenring" — the devices the village lends to the village.
///
/// The catalogue is public: it is here before anybody signs in, and it stays
/// here when the sign-in has gone stale. Booking is the part that needs the
/// Rössing-ID, and it says so where it is needed, not in front of the door.
struct RentalCatalogView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @State private var model: RentalCatalogModel?

    var body: some View {
        Group {
            if let model {
                RentalCatalogList(model: model)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Maschinchenring")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let existing = model ?? RentalCatalogModel(source: .from(umgebung.rental))
            model = existing
            await existing.load()
        }
    }
}

/// The list itself, once the model stands.
struct RentalCatalogList: View {
    @ObservedObject var model: RentalCatalogModel

    var body: some View {
        List {
            // The note first, then the (possibly older) list. An empty page
            // without an explanation would be the worst result.
            if let hint = model.hint {
                Section {
                    RentalNotice(text: hint) {
                        Task { await model.refresh() }
                    }
                }
            }

            Section {
                if model.loading && model.devices.isEmpty {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Geräte werden geholt …").foregroundStyle(.secondary)
                    }
                    .accessibilityIdentifier("rental-loading")
                }

                ForEach(model.visible) { device in
                    NavigationLink {
                        RentalDeviceView(device: device)
                    } label: {
                        RentalDeviceRow(device: device)
                    }
                    .accessibilityIdentifier("rental-device-\(device.id)")
                }

                if model.withoutMatch {
                    Text("Dazu gibt es kein Gerät. Versuch es mit einem anderen Wort.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("rental-no-match")
                }

                if model.empty {
                    Text("Im Maschinchenring steht gerade nichts zum Ausleihen. "
                        + "Sobald jemand ein Gerät einstellt, steht es hier.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("rental-empty")
                }
            } footer: {
                Text("Die Geräte gehören Leuten aus dem Dorf. Gepflegt werden sie "
                    + "im Maschinchenring — hier stehen sie nur.")
            }

            if model.showsSets {
                setsSection
            }

            mineSection
        }
        .accessibilityIdentifier("rental-catalog")
        .searchable(text: $model.query, prompt: "Gerät suchen")
        .task(id: model.query) {
            // Wait until the typing has settled, then ask the platform: its
            // search weighs meaning and wording together, and asking on every
            // keystroke would be a lot of noise for one word.
            try? await Task.sleep(nanoseconds: 300_000_000)
            if Task.isCancelled { return }
            await model.runSearch()
        }
        .refreshable { await model.refresh() }
    }

    /// Sets are shown, not booked. Cancelling, confirming and turning down a
    /// set booking is not implemented on the server yet, so the app does not
    /// lead into it either (`docs/mieten-api.md`, route 4).
    private var setsSection: some View {
        Section {
            ForEach(model.sets) { set in
                VStack(alignment: .leading, spacing: 4) {
                    Text(set.name).font(.headline)
                    if let text = set.description {
                        Text(text)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    ForEach(set.tariffs, id: \.self) { tariff in
                        Text(tariff)
                            .font(.footnote.weight(.medium))
                            .foregroundStyle(.tint)
                    }
                    if let deposit = set.depositText {
                        Text(deposit).font(.footnote).foregroundStyle(.secondary)
                    }
                }
                .padding(.vertical, 2)
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("rental-set-\(set.id)")
            }
        } header: {
            Text("Sets")
        } footer: {
            Text("Sets werden im Maschinchenring gebucht — in der App stehen sie "
                + "nur zur Übersicht.")
        }
    }

    private var mineSection: some View {
        Section("Meins") {
            NavigationLink {
                RentalBookingsView()
            } label: {
                Label("Meine Buchungen", systemImage: "calendar.badge.clock")
            }
            .accessibilityIdentifier("rental-my-bookings")

            NavigationLink {
                RentalProfileView()
            } label: {
                Label("Mein Profil im Maschinchenring", systemImage: "person.text.rectangle")
            }
            .accessibilityIdentifier("rental-profile")
        }
    }
}

/// One row of the catalogue.
struct RentalDeviceRow: View {
    let device: RentalDevice

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            RentalThumbnail(url: device.thumbnailURL)

            VStack(alignment: .leading, spacing: 3) {
                Text(device.name)
                    .font(.headline)
                    .fixedSize(horizontal: false, vertical: true)

                if !device.summary.isEmpty {
                    Text(device.summary)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }

                // The first tariff the platform has. Never added up with the
                // others: which one applies to which duration is nowhere
                // written down.
                if let first = device.tariffs.first {
                    Text(first)
                        .font(.footnote.weight(.medium))
                        .foregroundStyle(.tint)
                }

                if !device.tags.isEmpty {
                    Text(device.tags.joined(separator: " · "))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                if device.active == false {
                    Text("Abgeschaltet — für andere unsichtbar.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}
